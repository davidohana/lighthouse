package mcs

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// ExportSpec holds the relevant data for creating imports and arbitrating conflicts
// in service definitions.
type ExportSpec struct {
	// CreatedAt is the API master assigned creation time for the ServiceExport, used in
	// conflict resolution.
	CreatedAt metav1.Time `json:"createdAt"`
	// ClusterID indicates the origin cluster represented by the ServiceExport.
	ClusterID string `json:"clusterID"`
	// Namespace indicates the original ServiceExport object namespace.
	Namespace string `json:"ns"`
	// Name indicates the original ServiceExport object name.
	Name string `json:"name"`
	// Service includes the global properties of the exported Service
	Service GlobalProperties `json:"globalProperties"`
}

// GlobalProperties holds the global Service properties as defined by KEP-1645.
// See https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/1645-multi-cluster-services-api#global-properties.
type GlobalProperties struct {
	// Type set for Service (one of ClusterSetIP or Headless).
	Type mcsv1a1.ServiceImportType `json:"type"`
	// SessionAffinity set for the Service.
	SessionAffinity corev1.ServiceAffinity `json:"affinity,omitempty"`
	// SessionAffinityConfig set for the Service.
	SessionAffinityConfig *corev1.SessionAffinityConfig `json:"affinityConfig,omitempty"`
	// Ports defined for the Service.
	Ports []mcsv1a1.ServicePort `json:"ports"`
}

const (
	prefix                  = "lighthouse.submariner.io"
	ServiceExportAnnotation = prefix + "/" + "serviceExportSpec"
)

// NewExportSpec creates a new ExportSpec based on the given Service and
// ServiceExport.
func NewExportSpec(svc *corev1.Service, export *mcsv1a1.ServiceExport,
	cluster string) (*ExportSpec, error) {

	if svc == nil || export == nil || cluster == "" ||
		svc.Namespace != export.Namespace || svc.Name != export.Name {
		return nil, os.ErrInvalid
	}

	es := &ExportSpec{
		CreatedAt: export.GetCreationTimestamp(),
		ClusterID: cluster,
		Namespace: export.Namespace,
		Name:      export.Name,
		Service: GlobalProperties{
			Type:                  mcsv1a1.ClusterSetIP,
			SessionAffinity:       svc.Spec.SessionAffinity,
			SessionAffinityConfig: svc.Spec.SessionAffinityConfig,
		},
	}

	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		es.Service.Type = mcsv1a1.Headless
	}

	for _, p := range svc.Spec.Ports {
		es.Service.Ports = append(es.Service.Ports, mcsv1a1.ServicePort{
			Port:        p.Port,
			Name:        p.Name,
			Protocol:    p.Protocol,
			AppProtocol: p.AppProtocol,
		})
	}
	return es, nil
}

// MarshalObjectMeta encodes the ExportSpec object into the given
// ObjectMeta's annotations. Returns nil when successfully encoded and
// an error otherwise.
// Note: there is currently duplication of information (e.g., cluster,
// namespace, name) between the annotation created and the annotations
// used by Lighthouse's sync framework.
// @todo: use UTC as the canonical time zone
func (es *ExportSpec) MarshalObjectMeta(md *metav1.ObjectMeta) error {
	if es == nil || md == nil {
		return os.ErrInvalid
	}

	b, err := json.Marshal(es)
	if err != nil {
		return err
	}

	var bb bytes.Buffer
	if err = json.Compact(&bb, b); err != nil {
		return err
	}

	metav1.SetMetaDataAnnotation(md, ServiceExportAnnotation, bb.String())
	return nil
}

// UnmarshalObjectMeta decodes the ExportSpec object from the given
// ObjectMeta's annotations. Returns nil when successfully decoded and
// an error otherwise.
func (es *ExportSpec) UnmarshalObjectMeta(md *metav1.ObjectMeta) error {
	if es == nil || md == nil {
		return os.ErrInvalid
	}

	value := md.Annotations[ServiceExportAnnotation]
	if value == "" {
		return os.ErrNotExist
	}

	return json.Unmarshal([]byte(value), es)
}

// IsPrefferredOver determines if this ExportSpec has precedence over the given
// ExportSpec. This is based on the conflict resolution specification set in the
// KEP (i.e.,earliest creation time wins), extended for consistency in the case
// where exports are created at the exact same time.
func (es *ExportSpec) IsPrefferredOver(another *ExportSpec) bool {
	if es.CreatedAt == another.CreatedAt {
		return es.ClusterID <= another.ClusterID
	}
	return es.CreatedAt.Before(&another.CreatedAt)
}

// IsCompatibleWith determines if the ExportSpec are compatible or not.
// Retruns true when the specifications are compatible, otherwise returns
// false and the name of the conflicting field.
// @todo do we want to collect a map of the conflicting Global Properties?
func (es *ExportSpec) IsCompatibleWith(another *ExportSpec) (bool, string) {
	if es.Service.Type != another.Service.Type {
		return false, "type"
	} else if es.Service.SessionAffinity != another.Service.SessionAffinity {
		return false, "affinity"
	} else if !reflect.DeepEqual(es.Service.SessionAffinityConfig, another.Service.SessionAffinityConfig) {
		return false, "affinityConfig"
	}

	ports := make(map[string]mcsv1a1.ServicePort, len(es.Service.Ports))
	for _, p := range es.Service.Ports {
		ports[p.Name] = p
	}

	for _, other := range another.Service.Ports {
		current, found := ports[other.Name]

		if !found {
			continue
		} else if !reflect.DeepEqual(current, other) {
			return false, "ports"
		}
	}
	return true, ""
}
