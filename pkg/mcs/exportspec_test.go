package mcs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/submariner-io/lighthouse/pkg/mcs"
)

var (
	timeout = int32(10)

	sample = mcs.ExportSpec{
		CreatedAt: metav1.NewTime(time.Now().UTC()),
		ClusterID: "cluster",
		Namespace: "namespace",
		Name:      "name",
		Service: mcs.GlobalProperties{
			Type:            mcsv1a1.ClusterSetIP,
			SessionAffinity: corev1.ServiceAffinityClientIP,
			SessionAffinityConfig: &corev1.SessionAffinityConfig{
				ClientIP: &corev1.ClientIPConfig{
					TimeoutSeconds: &timeout,
				},
			},
			Ports: []mcsv1a1.ServicePort{
				{Port: 80, Name: "http", Protocol: corev1.ProtocolTCP},
			},
		},
	}
)

func TestMarshalUnmarshal(t *testing.T) {
	testcases := map[string]struct {
		expected *mcs.ExportSpec
		mutate   func(mcs.ExportSpec) *mcs.ExportSpec
	}{
		/*
			"empty": {},
			"missing: timestamp": {},
			"missing: cluster": {},
			"missing: namespace": {},
			"missing: name": {},
			"missing: service":       {},
			"missing: service.type":  {},
			"missing: service.ports": {},
			"simple spec": {},
			"multiple ports":         {},
		*/
	}

	assertions := require.New(t)

	for name, test := range testcases {
		t.Logf("Running test case %s", name)

		md := &metav1.ObjectMeta{}
		test.expected = test.mutate(sample)
		err := sample.MarshalObjectMeta(md)
		assertions.NoError(err)

		actual := &mcs.ExportSpec{}
		err = actual.UnmarshalObjectMeta(md)
		assertions.NoError(err)

		assertions.Equal(test.expected, actual)
	}
}

func TestCompatibility(t *testing.T) {
	testcases := map[string]struct {
		spec       *mcs.ExportSpec
		other      *mcs.ExportSpec
		compatible bool
		field      string
	}{
		"compatible: empty": {spec: &mcs.ExportSpec{}, other: &mcs.ExportSpec{}, compatible: true},
		/*
			"compatible: identical":        {},
			"compatible: local properties": {},
			"conflicting: type":            {},
			"conflicting: affinity":        {},
			"conflicting: port":            {},
		*/
	}

	assertions := require.New(t)

	for name, test := range testcases {
		t.Logf("Running test case %s", name)

		compatible, field := test.spec.IsCompatibleWith(test.other)
		assertions.Equal(test.compatible, compatible)
		assertions.Equal(test.field, field, "unexpected field conflict detected")
	}
}

func TestConflictResolutionCriteria(t *testing.T) {
	testcases := map[string]struct {
		spec      *mcs.ExportSpec
		other     *mcs.ExportSpec
		preferred bool
	}{
		"different time, prefer earlier": {
			spec: &mcs.ExportSpec{
				CreatedAt: metav1.Now(),
			},
			other: &mcs.ExportSpec{
				CreatedAt: metav1.NewTime(time.Now().Add(1 * time.Second)),
			},
			preferred: true,
		},
		"equal time, prefer lesser cluster name": {
			spec: &mcs.ExportSpec{
				CreatedAt: metav1.Unix(0, 0),
				ClusterID: "cluster1",
			},
			other: &mcs.ExportSpec{
				CreatedAt: metav1.Unix(0, 0),
				ClusterID: "cluster2",
			},
			preferred: true,
		},
	}

	assertions := require.New(t)

	for name, test := range testcases {
		t.Logf("Running test case %s", name)

		preferred := test.spec.IsPrefferredOver(test.other)
		assertions.True(test.preferred == preferred || test.spec == test.other)
		reversed := test.other.IsPrefferredOver(test.spec)
		assertions.True(preferred == !reversed || test.spec == test.other)
	}
}

func TestCreateFromObjects(t *testing.T) {
	t.Log("not implemented")
}
