/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
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
		//Truncate time to UTC time-zone with seconds resulotion in order to be compatible with k8s marshal returned time
		CreatedAt: metav1.NewTime(time.Now().UTC().Truncate(time.Second)),
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

		"empty": {
			expected: &mcs.ExportSpec{},
			mutate:   func(mcs.ExportSpec) *mcs.ExportSpec { return &mcs.ExportSpec{} },
		},

		"missing: timestamp": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.CreatedAt = metav1.Unix(0, 0)
				return &es
			},
		},

		"missing: cluster": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.ClusterID = ""
				return &es
			},
		},

		"missing: namespace": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Namespace = ""
				return &es
			},
		},

		"missing: name": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Name = ""
				return &es
			},
		},

		"missing: service": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service = mcs.GlobalProperties{}
				return &es
			},
		},

		"missing: service.type": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service.Type = ""
				return &es
			},
		},

		"missing: service.ports": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service.Ports = []mcsv1a1.ServicePort{}
				return &es
			},
		},

		"simple spec": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				return &es
			},
		},

		"multiple ports": {
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				new_port_81 := mcsv1a1.ServicePort{Port: 81, Name: "http2", Protocol: corev1.ProtocolTCP}
				new_port_82 := mcsv1a1.ServicePort{Port: 82, Name: "http3", Protocol: corev1.ProtocolTCP}
				es.Service.Ports = append(es.Service.Ports, new_port_81)
				es.Service.Ports = append(es.Service.Ports, new_port_82)
				return &es
			},
		},
	}

	assertions := require.New(t)

	for name, test := range testcases {
		t.Logf("Running test case %s", name)
		md := &metav1.ObjectMeta{}
		test.expected = test.mutate(sample)
		err := test.expected.MarshalObjectMeta(md)
		assertions.NoError(err)
		actual := &mcs.ExportSpec{}
		err = actual.UnmarshalObjectMeta(md)
		assertions.NoError(err)
		test.expected.CreatedAt.Time = test.expected.CreatedAt.UTC()
		actual.CreatedAt.Time = actual.CreatedAt.Time.UTC()
		assertions.Equal(test.expected, actual)
	}
}

func TestCompatibility(t *testing.T) {
	testcases := map[string]struct {
		spec       *mcs.ExportSpec
		other      *mcs.ExportSpec
		compatible bool
		field      string
		mutate     func(mcs.ExportSpec) *mcs.ExportSpec
	}{
		"compatible: empty": {spec: &mcs.ExportSpec{}, compatible: true,
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				return &mcs.ExportSpec{}
			}},

		"compatible: identical": {spec: &sample, compatible: true,
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				return &es
			}},

		"compatible: cluserID": {spec: &sample, compatible: true,
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.ClusterID = "different_cluster"
				return &es
			}},
		"compatible: CreatedAt": {spec: &sample, compatible: true,
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.CreatedAt.Time = es.CreatedAt.Time.Add(1 * time.Second)
				return &es
			}},
		"conflicting: name": {spec: &sample, compatible: false, field: "name",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Name = "different_name"
				return &es
			}},
		"conflicting: namespace": {spec: &sample, compatible: false, field: "namespace",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Namespace = "different_namepsace"
				return &es
			}},

		"conflicting: type": {spec: &sample, compatible: false, field: "type",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service.Type = mcsv1a1.Headless
				return &es
			}},

		"conflicting: affinity": {spec: &sample, compatible: false, field: "affinity",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service.SessionAffinity = corev1.ServiceAffinityNone
				return &es
			}},

		"conflicting: affinity config": {spec: &sample, compatible: false, field: "affinity config",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				otherTimeout := int32(11)
				es.Service.SessionAffinityConfig = &corev1.SessionAffinityConfig{
					ClientIP: &corev1.ClientIPConfig{
						TimeoutSeconds: &otherTimeout,
					},
				}
				return &es
			}},

		"conflicting: port": {spec: &sample, compatible: false, field: "ports",
			mutate: func(es mcs.ExportSpec) *mcs.ExportSpec {
				es.Service.Ports = append(es.Service.Ports, mcsv1a1.ServicePort{Port: 81, Name: "http", Protocol: corev1.ProtocolTCP})
				return &es
			}},
	}

	assertions := require.New(t)

	for name, test := range testcases {
		t.Logf("Running test case %s", name)

		other := test.mutate(*test.spec)
		err := test.spec.IsCompatibleWith(other)

		if test.compatible {
			assertions.Nil(err)
		} else {
			assertions.NotNil(err)
			assertions.NotEqual("", err.Error())
			assertions.Equal(err.Cause(), test.field)
		}
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

		preferred := test.spec.IsPreferredOver(test.other)
		assertions.True(test.preferred == preferred || test.spec == test.other)
		reversed := test.other.IsPreferredOver(test.spec)
		assertions.True(preferred == !reversed || test.spec == test.other)
	}
}

func TestCreateFromObjects(t *testing.T) {
	t.Log("not implemented")
}
