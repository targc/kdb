package redis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "kdb.io", Version: "v1alpha1"}

func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Redis{}, &RedisList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

type Spec struct {
	Domain   string `json:"domain"`
	Password string `json:"password,omitempty"`
	Image    string `json:"image,omitempty"`
}

type Status struct {
	Phase string `json:"phase,omitempty"`
}

type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec,omitempty"`
	Status            Status `json:"status,omitempty"`
}

type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func (r *Redis) DeepCopyObject() runtime.Object {
	out := new(Redis)
	r.DeepCopyInto(out)
	return out
}

func (r *Redis) DeepCopyInto(out *Redis) {
	*out = *r
	r.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = r.Spec
}

func (rl *RedisList) DeepCopyObject() runtime.Object {
	out := new(RedisList)
	rl.DeepCopyInto(out)
	return out
}

func (rl *RedisList) DeepCopyInto(out *RedisList) {
	*out = *rl
	rl.ListMeta.DeepCopyInto(&out.ListMeta)
	if rl.Items != nil {
		out.Items = make([]Redis, len(rl.Items))
		for i := range rl.Items {
			rl.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
