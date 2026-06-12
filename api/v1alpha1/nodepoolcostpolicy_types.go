package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolCostPolicySpec defines the desired state of NodePoolCostPolicy.
type NodePoolCostPolicySpec struct {
	// clusterID is the DigitalOcean Kubernetes cluster UUID.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterID string `json:"clusterID"`

	// poolID is the DigitalOcean node pool UUID to monitor.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PoolID string `json:"poolID"`

	// nodeSize is a human-readable label for the node size (e.g. "s-8vcpu-16gb").
	// Used only in Slack messages for context.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NodeSize string `json:"nodeSize"`

	// hourlyCostPerNode is the DigitalOcean hourly rate for a single node of this size, in USD.
	// Monthly estimate = nodeCount * hourlyCostPerNode * 730.
	// +kubebuilder:validation:Required
	HourlyCostPerNode float64 `json:"hourlyCostPerNode"`

	// monthlyCap is the maximum acceptable monthly cost in USD for this pool.
	// An alert fires at 80% and again at 100%.
	// +kubebuilder:validation:Required
	MonthlyCap float64 `json:"monthlyCap"`
}

// NodePoolCostPolicyStatus defines the observed state of NodePoolCostPolicy.
type NodePoolCostPolicyStatus struct {
	// currentNodeCount is the node count reported by the DigitalOcean API on the last poll.
	// +optional
	CurrentNodeCount int `json:"currentNodeCount,omitempty"`

	// currentMonthlyCost is the estimated monthly cost in USD based on the last poll.
	// Calculated as currentNodeCount * hourlyCostPerNode * 730.
	// +optional
	CurrentMonthlyCost float64 `json:"currentMonthlyCost,omitempty"`

	// percentOfCap is currentMonthlyCost / monthlyCap * 100, rounded to two decimal places.
	// +optional
	PercentOfCap float64 `json:"percentOfCap,omitempty"`

	// lastAlertLevel is the threshold level at which the most recent threshold alert was sent.
	// One of: "" (none), "warning" (>=80%), "critical" (>=100%).
	// +optional
	LastAlertLevel string `json:"lastAlertLevel,omitempty"`

	// lastAlertSentAt is the timestamp at which the last threshold alert was sent.
	// +optional
	LastAlertSentAt *metav1.Time `json:"lastAlertSentAt,omitempty"`

	// lastDigestSentAt is the timestamp at which the last daily digest was sent.
	// +optional
	LastDigestSentAt *metav1.Time `json:"lastDigestSentAt,omitempty"`

	// lastPollTime is the timestamp of the most recent successful DigitalOcean API poll.
	// +optional
	LastPollTime *metav1.Time `json:"lastPollTime,omitempty"`

	// conditions represent the current state of the NodePoolCostPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.poolID`
// +kubebuilder:printcolumn:name="Cap ($)",type=number,JSONPath=`.spec.monthlyCap`
// +kubebuilder:printcolumn:name="Est. Cost ($)",type=number,JSONPath=`.status.currentMonthlyCost`
// +kubebuilder:printcolumn:name="% of Cap",type=number,JSONPath=`.status.percentOfCap`
// +kubebuilder:printcolumn:name="Alert",type=string,JSONPath=`.status.lastAlertLevel`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodePoolCostPolicy is the Schema for the nodepoolcostpolicies API.
type NodePoolCostPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolCostPolicySpec   `json:"spec"`
	Status NodePoolCostPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolCostPolicyList contains a list of NodePoolCostPolicy.
type NodePoolCostPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePoolCostPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePoolCostPolicy{}, &NodePoolCostPolicyList{})
}