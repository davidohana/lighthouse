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
package controller_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/syncer/test"
	"github.com/submariner-io/lighthouse/pkg/agent/controller"
	"github.com/submariner-io/lighthouse/pkg/lhutil"
	"github.com/submariner-io/lighthouse/pkg/mcs"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("ServiceExport syncing", func() {
	var t *testDriver

	BeforeEach(func() {
		t = newTestDriver()
	})

	JustBeforeEach(func() {
		t.justBeforeEach()
	})

	AfterEach(func() {
		t.afterEach()
	})

	When("a ServiceExport is created", func() {
		When("the Service already exists", func() {
			It("should update the ServiceExport status and upload to the broker", func() {
				t.awaitNoServiceExportOnBroker()
				t.createService()
				t.createLocalServiceExport()
				t.awaitServiceExported()
			})
		})

		When("the Service doesn't initially exist", func() {
			It("should initially update the ServiceExport status to not Valid and upload after service is created", func() {
				t.createLocalServiceExport()
				t.awaitServiceUnavailableStatus()
				t.awaitNoServiceExportOnBroker()

				t.createService()
				t.awaitServiceExported()
			})
		})
	})

	When("a ServiceExport is deleted after it was synced", func() {
		It("should delete the ServiceExport on the Broker", func() {
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()

			t.deleteLocalServiceExport()
			t.awaitNoServiceExportOnBroker()
		})
	})

	When("an exported Service is deleted and recreated while the ServiceExport still exists", func() {
		It("should delete and recreate the ServiceExport on the broker", func() {
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()

			t.deleteService()
			t.awaitNoServiceExportOnBroker()
			t.awaitServiceUnavailableStatus()

			t.createService()
			t.awaitServiceExported()
		})
	})

	When("the ServiceExport sync initially fails", func() {
		BeforeEach(func() {
			t.brokerServiceExportClient.PersistentFailOnCreate.Store("mock create error")
		})

		It("should not update the ServiceExport status to Exported until the sync is successful", func() {
			t.createService()
			t.createLocalServiceExport()

			condition := newServiceExportValidityCondition(corev1.ConditionFalse, controller.ReasonAwaitingSync)
			t.awaitLocalServiceExport(condition)

			t.awaitNoServiceExportOnBroker()

			t.brokerServiceExportClient.PersistentFailOnCreate.Store("")
			t.awaitServiceExported()
		})
	})

	When("the ServiceExportCondition list count reaches MaxExportStatusConditions", func() {
		var oldMaxExportStatusConditions int

		BeforeEach(func() {
			oldMaxExportStatusConditions = controller.MaxExportStatusConditions
			controller.MaxExportStatusConditions = 1
		})

		AfterEach(func() {
			controller.MaxExportStatusConditions = oldMaxExportStatusConditions
		})

		It("should correctly truncate the ServiceExportCondition list", func() {
			t.createService()
			t.createLocalServiceExport()

			t.awaitServiceExported()

			serviceExport := t.awaitLocalServiceExport(nil)
			Expect(len(serviceExport.Status.Conditions)).To(Equal(1))
		})
	})

	When("a ServiceExport is created for a Service whose type is other than ServiceTypeClusterIP", func() {
		BeforeEach(func() {
			t.service.Spec.Type = corev1.ServiceTypeNodePort
		})

		It("should update the ServiceExport status and not sync it", func() {
			t.createService()
			t.createLocalServiceExport()

			condition := newServiceExportValidityCondition(corev1.ConditionFalse, controller.ReasonUnsupportedServiceType)
			t.awaitLocalServiceExport(condition)

			t.awaitNoServiceExportOnBroker()
		})
	})

	When("a Service has port information", func() {
		BeforeEach(func() {
			t.service.Spec.Ports = []corev1.ServicePort{
				{
					Name:     "port_name1",
					Protocol: corev1.ProtocolTCP,
					Port:     123,
				},
				{
					Name:     "port_name2",
					Protocol: corev1.ProtocolSCTP,
					Port:     1234,
				},
			}
		})

		It("should set the appropriate port information in the ServiceExport", func() {
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported() // t.service.Spec.ClusterIP, 0

			serviceExport := t.awaitBrokerServiceExport(nil)
			exportSpec := &mcs.ExportSpec{}
			err := exportSpec.UnmarshalObjectMeta(&serviceExport.ObjectMeta)
			Expect(err).To(BeNil())
			Expect(len(exportSpec.Service.Ports)).To(Equal(2))
			Expect(exportSpec.Service.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(exportSpec.Service.Ports[1].Protocol).To(Equal(corev1.ProtocolSCTP))
			Expect(exportSpec.Service.Ports[0].Port).To(BeNumerically("==", 123))
			Expect(exportSpec.Service.Ports[1].Port).To(BeNumerically("==", 1234))
			Expect(time.Since(exportSpec.CreatedAt.Time)).To(BeNumerically("<", 60*time.Second))
		})
	})

	// todo: fix test
	// this test does not test what it should because UpdateStatus() is not affected by FailOnUpdate
	// so in practice, no fake conflict is produced
	When("a conflict initially occurs when updating the ServiceExport status", func() {
		BeforeEach(func() {
			err := apierrors.NewConflict(schema.GroupResource{}, t.serviceExport.Name, errors.New("fake conflict"))
			t.cluster1.serviceExportClient.FailOnUpdate = err
		})

		It("should eventually update the ServiceExport status", func() {
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()
		})
	})

	When("hub updates service export conflict status", func() {
		It("only condition of conflict status shall be updated on local export", func() {
			logger.Info("Sync export to broker")
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()
			brokerExport := t.awaitBrokerServiceExport(nil)

			logger.Info("Create a condition other than conflict on broker - should not be downloaded to spoke")
			cond := lhutil.CreateServiceExportCondition(mcsv1a1.ServiceExportValid,
				corev1.ConditionFalse, "other reason", "other message")
			t.addBrokerServiceExportCondition(brokerExport, cond)
			time.Sleep(300 * time.Millisecond)
			localServiceExport := t.awaitLocalServiceExport(nil)
			t.awaitServiceExported()
			Expect(lhutil.GetServiceExportCondition(&localServiceExport.Status, mcsv1a1.ServiceExportConflict)).To(BeNil())

			logger.Info("Create conflict condition - should be downloaded")
			cond =
				lhutil.CreateServiceExportCondition(mcsv1a1.ServiceExportConflict,
					corev1.ConditionTrue, "protocol conflict", "export conflict found")
			t.addBrokerServiceExportCondition(brokerExport, cond)
			t.awaitBrokerServiceExport(cond)
			t.awaitLocalServiceExport(cond)

			logger.Info("Resolve conflict condition - should be downloaded")
			cond = lhutil.CreateServiceExportCondition(mcsv1a1.ServiceExportConflict,
				corev1.ConditionFalse, "protocol conflict resolved", "export conflict resolved")
			t.addBrokerServiceExportCondition(brokerExport, cond)
			t.awaitBrokerServiceExport(cond)
			t.awaitLocalServiceExport(cond)
		})
	})

	When("local service export is deleted out of band after sync", func() {
		It("should delete it from the broker datastore on reconciliation", func() {
			// simulate sync to broker to get the export state as it should be on the broker
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()
			brokerServiceExport := t.awaitBrokerServiceExport(nil)

			t.afterEach()                                                         // stop agent controller on all clusters
			t = newTestDriver()                                                   // create a new driver - data stores are now empty
			test.CreateResource(t.brokerServiceExportClient, brokerServiceExport) // create export on broker only
			t.createService()                                                     // recreate service so that only local export is missing
			t.justBeforeEach()                                                    // start agent controller on all clusters
			t.awaitNoServiceExportOnBroker()                                      // ensure that the export is deleted from broker
			t.createLocalServiceExport()                                          // recreate local export online
			t.awaitServiceExported()                                              // ensure export is synced to broker
		})
	})

	When("local service is deleted out of band after sync", func() {
		It("should delete export from the broker datastore on reconciliation", func() {
			// simulate sync to broker to get the export state as it should be on the broker
			t.createService()
			t.createLocalServiceExport()
			t.awaitServiceExported()
			brokerServiceExport := t.awaitBrokerServiceExport(nil)

			t.afterEach()                                                         // stop agent controller on all clusters
			t = newTestDriver()                                                   // create a new driver - data stores are now empty
			test.CreateResource(t.brokerServiceExportClient, brokerServiceExport) // create export on broker only

			t.justBeforeEach()               // start agent controller on all clusters, both export and service are missing
			t.awaitNoServiceExportOnBroker() // ensure that the export is deleted from broker
			t.createLocalServiceExport()     // recreate local export online
			t.createService()                // recreate local service online
			t.awaitServiceExported()         // ensure export is synced to broker
		})
	})

	When("local service and service export are created out of band", func() {
		It("should sync export to the broker datastore on reconciliation", func() {
			t.afterEach()                // stop agent controller on all clusters
			t = newTestDriver()          // create a new driver - data stores are now empty
			t.createService()            // create local service oob
			t.createLocalServiceExport() // recreate local export online
			t.justBeforeEach()           // start agent controller on all clusters, both export and service are missing
			t.awaitServiceExported()     // ensure export is synced to broker
		})
	})
})
