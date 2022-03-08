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

package main

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/submariner-io/lighthouse/pkg/lhutil"
)

const (
	serviceExportFinalizerName = "lighthouse.submariner.io/service-export-finalizer"
)

// ServiceExportReconciler reconciles a ServiceExport object
type ServiceExportReconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/finalizers,verbs=get;update;delete
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/status,verbs=get;update
// +kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports,verbs=create;get;list;update;patch;delete

// Reconcile handles an update to a ServiceExport
// By design, the event that triggered reconciliation is not passed to the reconciler
// to ensure actions are based on state alone. This approach is referred to as level-based,
// as opposed to edge-based.
func (r *ServiceExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log := r.Log.WithValues("serviceexport", req.NamespacedName)

	// @todo extract and log the real (i.e., source) object name, namespace and cluster?
	log.Info("Reconciling ServiceExport")

	se := &mcsv1a1.ServiceExport{}
	if err := r.Client.Get(ctx, req.NamespacedName, se); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("No ServiceExport found")
		} else {
			// @tdo avoid requeuing for now. Revisit once we see what errors do show.
			log.Error(err, "Error fetching ServiceExport")
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(se, serviceExportFinalizerName) && se.GetDeletionTimestamp() == nil {
		controllerutil.AddFinalizer(se, serviceExportFinalizerName)
		if err := r.Client.Update(ctx, se); err != nil {
			if apierrors.IsConflict(err) { // The SE has been updated since we read it, requeue to retry reconciliation.
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Error adding finalizer")
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcile(ctx, lhutil.GetOriginalObjectName(se.ObjectMeta))
	if err != nil {
		return result, err
	}

	if se.GetDeletionTimestamp() != nil { // service export is marked to be deleted
		return r.handleDelete(ctx, se)
	}

	return result, err
}

// reconcile the cluster state.
//
// Reconciliation logic:
// 1. list all relevant exports (i.e., having the same name and namespace labels)
// 2. determine the primary export object
// 3. iterate on all relevant export objects
// 3a. if the object is being deleted (non empty deletion timestamp) - ignore
// 3b. if object conflicts with primary - update export.Status.Condition.Conflict
//		and delete corresponding ServiceImport.
// 3c. otherwise, create/update the corrresponding ServiceImport and set the
//		ServiceExport as owner.
//
// Notes:
// - ensure objects are protected with a finalizer. Since we only receive the
//	 affected object name into the reconciliation loop, we have no way to determine
//	 and retrieve affected ServiceExports (i.e., client.List would not return the
//   deleted object and we can't access its labels). The finalizer is cleared when we
// 	 receive a notification for change in an object scheduled for deletion.
// - ServiceImports are owned by the corresponding ServiceExport and are thus
//	 automatically deleted by k8s when their ServiceExport is deleted.
//
func (r *ServiceExportReconciler) reconcile(ctx context.Context, name types.NamespacedName) (ctrl.Result, error) {
	_ = r.Log.WithValues("service", name)

	//------------------------------------------
	// -- @TODO CODE ABOVE SPECIFIED BEHAVIOR --
	//------------------------------------------

	return ctrl.Result{}, nil
}

// allow the object to be deleted by removing the finalizer.
// Once all finalizers have been removed, the ServiceExport object and its
// owned ServiceImport object will be deleted.
//
// Note: ownership will need to be reconsidered if merging imports
func (r *ServiceExportReconciler) handleDelete(ctx context.Context, se *mcsv1a1.ServiceExport) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(se, serviceExportFinalizerName) {
		// @todo: add the NamespacedName of the ServiceExport to be compatible with Reconcile?
		log := r.Log.WithValues("service", lhutil.GetOriginalObjectName(se.ObjectMeta)).
			WithValues("cluster", lhutil.GetOriginalObjectCluster(se.ObjectMeta))

		log.Info("Removing service export finalizer")

		// clear the finalizer
		controllerutil.RemoveFinalizer(se, serviceExportFinalizerName)
		if err := r.Client.Update(ctx, se); err != nil {
			// @todo handle version mismatch error?
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the manager.
func (r *ServiceExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcsv1a1.ServiceExport{}).
		Owns(&mcsv1a1.ServiceImport{}).
		Complete(r)
}

// create or update the ServiceImport corresponding to the provided export.
func (r *ServiceExportReconciler) ensureImportFor(se *mcsv1a1.ServiceExport) error {
	//---------------------
	//-- @TODO IMPLEMENT --
	//---------------------
	return nil
}
