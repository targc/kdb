package mongo

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "kdb.io", Version: "v1alpha1"}

func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &Mongo{}, &MongoList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

type StorageSpec struct {
	PVCName      string   `json:"pvcName"`
	Size         string   `json:"size"`
	StorageClass string   `json:"storageClass"`
	AccessModes  []string `json:"accessModes"`
	MountPath    string   `json:"mountPath,omitempty"`
}

type Spec struct {
	Domain   string      `json:"domain"`
	User     string      `json:"user"`
	Password string      `json:"password"`
	Image    string      `json:"image,omitempty"`
	Storage  StorageSpec `json:"storage"`
}

type Status struct {
	Phase string `json:"phase,omitempty"`
}

type Mongo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec,omitempty"`
	Status            Status `json:"status,omitempty"`
}

type MongoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Mongo `json:"items"`
}

func (m *Mongo) DeepCopyObject() runtime.Object {
	out := new(Mongo)
	m.DeepCopyInto(out)
	return out
}

func (m *Mongo) DeepCopyInto(out *Mongo) {
	*out = *m
	m.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = m.Spec
}

func (ml *MongoList) DeepCopyObject() runtime.Object {
	out := new(MongoList)
	ml.DeepCopyInto(out)
	return out
}

func (ml *MongoList) DeepCopyInto(out *MongoList) {
	*out = *ml
	ml.ListMeta.DeepCopyInto(&out.ListMeta)
	if ml.Items != nil {
		out.Items = make([]Mongo, len(ml.Items))
		for i := range ml.Items {
			ml.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
