package postgres

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "kdb.io", Version: "v1alpha1"}

func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Postgres{}, &PostgresList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

type Spec struct {
	Domain   string `json:"domain"`
	User     string `json:"user"`
	Password string `json:"password"`
	Image    string `json:"image,omitempty"`
}

type Status struct {
	Phase string `json:"phase,omitempty"`
}

type Postgres struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec,omitempty"`
	Status            Status `json:"status,omitempty"`
}

type PostgresList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Postgres `json:"items"`
}

func (p *Postgres) DeepCopyObject() runtime.Object {
	out := new(Postgres)
	p.DeepCopyInto(out)
	return out
}

func (p *Postgres) DeepCopyInto(out *Postgres) {
	*out = *p
	p.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = p.Spec
}

func (pl *PostgresList) DeepCopyObject() runtime.Object {
	out := new(PostgresList)
	pl.DeepCopyInto(out)
	return out
}

func (pl *PostgresList) DeepCopyInto(out *PostgresList) {
	*out = *pl
	pl.ListMeta.DeepCopyInto(&out.ListMeta)
	if pl.Items != nil {
		out.Items = make([]Postgres, len(pl.Items))
		for i := range pl.Items {
			pl.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
