package controller

import (
	"context"
	"fmt"
	"math"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/tabed23/doks-cost-enforcer-operator/api/v1alpha1"
	"github.com/tabed23/doks-cost-enforcer-operator/internal/do"
	"github.com/tabed23/doks-cost-enforcer-operator/internal/slack"
)

// NodePoolCostPolicyReconciler reconciles a NodePoolCostPolicy object.
type NodePoolCostPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.mahy.love,resources=nodepoolcostpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.mahy.love,resources=nodepoolcostpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mahy.love,resources=nodepoolcostpolicies/finalizers,verbs=update

func (r *NodePoolCostPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// ── Fetch CR ─────────────────────────────────────────────────────────────
	policy := &platformv1alpha1.NodePoolCostPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get policy: %w", err)
	}

	spec := policy.Spec

	// ── Read credentials from env ─────────────────────────────────────────────
	// DO_TOKEN and SLACK_WEBHOOK_URL are injected via envFrom in the Deployment.
	doToken := os.Getenv("DO_TOKEN")
	if doToken == "" {
		err := fmt.Errorf("DO_TOKEN env var is not set")
		log.Error(err, "missing env var")
		r.setCondition(policy, metav1.ConditionFalse, "MissingEnv", err.Error())
		_ = r.Status().Update(ctx, policy)
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}

	slackURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackURL == "" {
		err := fmt.Errorf("SLACK_WEBHOOK_URL env var is not set")
		log.Error(err, "missing env var")
		r.setCondition(policy, metav1.ConditionFalse, "MissingEnv", err.Error())
		_ = r.Status().Update(ctx, policy)
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}

	// ── Poll DigitalOcean ─────────────────────────────────────────────────────
	pool, err := do.NewClient(doToken).GetNodePool(ctx, spec.ClusterID, spec.PoolID)
	if err != nil {
		log.Error(err, "DO API error")
		r.setCondition(policy, metav1.ConditionFalse, "DOAPIError", err.Error())
		_ = r.Status().Update(ctx, policy)
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}

	now := metav1.Now()
	policy.Status.CurrentNodeCount = pool.Count
	policy.Status.LastPollTime = &now

	// ── Estimate cost ─────────────────────────────────────────────────────────
	estimated := float64(pool.Count) * spec.HourlyCostPerNode * hoursPerMonth
	pct := round2(estimated / spec.MonthlyCap * 100)
	policy.Status.CurrentMonthlyCost = round2(estimated)
	policy.Status.PercentOfCap = pct

	log.Info("cost computed",
		"pool", spec.PoolID,
		"nodes", pool.Count,
		"estimatedMonthly", policy.Status.CurrentMonthlyCost,
		"percentOfCap", pct,
	)

	params := slack.AlertParams{
		PolicyName:           policy.Name,
		PoolID:               spec.PoolID,
		NodeSize:             spec.NodeSize,
		NodeCount:            pool.Count,
		HourlyCostPerNode:    spec.HourlyCostPerNode,
		MonthlyCap:           spec.MonthlyCap,
		EstimatedMonthlyCost: policy.Status.CurrentMonthlyCost,
		PercentOfCap:         pct,
	}

	fraction := estimated / spec.MonthlyCap
	prev := policy.Status.LastAlertLevel

	// ── Threshold alerts ──────────────────────────────────────────────────────
	switch {
	case fraction >= critThreshold && prev != alertLevelCritical:
		log.Info("sending critical alert", "pool", spec.PoolID)
		if err := slack.SendCritical(ctx, slackURL, params); err != nil {
			log.Error(err, "failed to send critical alert")
		} else {
			policy.Status.LastAlertLevel = alertLevelCritical
			policy.Status.LastAlertSentAt = &now
		}

	case fraction >= warnThreshold && fraction < critThreshold && prev == alertLevelNone:
		log.Info("sending warning alert", "pool", spec.PoolID)
		if err := slack.SendWarning(ctx, slackURL, params); err != nil {
			log.Error(err, "failed to send warning alert")
		} else {
			policy.Status.LastAlertLevel = alertLevelWarning
			policy.Status.LastAlertSentAt = &now
		}

	case fraction < warnThreshold:
		// Recovered — reset so alerts fire again if cost climbs back up.
		policy.Status.LastAlertLevel = alertLevelNone
	}

	// ── Daily digest ──────────────────────────────────────────────────────────
	if policy.Status.LastDigestSentAt == nil ||
		now.Time.Sub(policy.Status.LastDigestSentAt.Time) >= digestInterval {

		log.Info("sending daily digest", "pool", spec.PoolID)
		if err := slack.SendDailyDigest(ctx, slackURL, params); err != nil {
			log.Error(err, "failed to send daily digest")
		} else {
			policy.Status.LastDigestSentAt = &now
		}
	}

	r.setCondition(policy, metav1.ConditionTrue, "Reconciled",
		fmt.Sprintf("est. $%.2f (%.1f%% of cap)", policy.Status.CurrentMonthlyCost, pct))

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("status update: %w", err)
	}

	return ctrl.Result{RequeueAfter: pollInterval}, nil
}

func (r *NodePoolCostPolicyReconciler) setCondition(
	policy *platformv1alpha1.NodePoolCostPolicy,
	status metav1.ConditionStatus,
	reason, message string,
) {
	now := metav1.Now()
	for i, c := range policy.Status.Conditions {
		if c.Type == conditionReady {
			policy.Status.Conditions[i].Status = status
			policy.Status.Conditions[i].Reason = reason
			policy.Status.Conditions[i].Message = message
			policy.Status.Conditions[i].LastTransitionTime = now
			return
		}
	}
	policy.Status.Conditions = append(policy.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: policy.Generation,
	})
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// SetupWithManager registers the controller with the Manager.
func (r *NodePoolCostPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.NodePoolCostPolicy{}).
		Named("nodepoolcostpolicy").
		Complete(r)
}
