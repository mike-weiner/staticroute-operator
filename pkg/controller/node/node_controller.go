//
// Copyright 2020 IBM Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package node

import (
	"context"

	iksv1 "github.com/IBM/staticroute-operator/pkg/apis/iks/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_node")

// Add creates a new Node Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNode{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("node-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Node
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNode implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNode{}

// ReconcileNode reconciles a Node object
type ReconcileNode struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Node object and makes changes based on the state read
// and what is in the Node.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// Fetch the Node instance
	node := &corev1.Node{}
	err := r.client.Get(context.Background(), request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			routes := &iksv1.StaticRouteList{}
			err := r.client.List(context.Background(), routes)
			if err != nil {
				reqLogger.Error(err, "Unable to fetch CRDs")
				return reconcile.Result{}, err
			}

			nf := nodeFinder{
				nodeName: request.Name,
				routes:   routes,
				updateCallback: func(route *iksv1.StaticRoute) error {
					return r.client.Update(context.Background(), route)
				},
				infoLogger: reqLogger.Info,
			}
			nf.findAndDeleteNode()
			if err != nil {
				reqLogger.Error(err, "Unable to update CRD")
				return reconcile.Result{}, err
			}
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

type nodeFinder struct {
	nodeName       string
	routes         *iksv1.StaticRouteList
	updateCallback func(*iksv1.StaticRoute) error
	infoLogger     func(string, ...interface{})
}

func (nf *nodeFinder) findAndDeleteNode() error {
	for _, route := range nf.routes.Items {
		statusToDelete := -1
		for i, status := range route.Status.NodeStatus {
			if status.Hostname != nf.nodeName {
				continue
			}
			statusToDelete = i
			break
		}
		if statusToDelete > -1 {
			nf.infoLogger("Found the node to delete")

			copy(route.Status.NodeStatus[statusToDelete:], route.Status.NodeStatus[statusToDelete+1:])
			route.Status.NodeStatus[len(route.Status.NodeStatus)-1] = iksv1.StaticRouteNodeStatus{}
			route.Status.NodeStatus = route.Status.NodeStatus[:len(route.Status.NodeStatus)-1]

			if err := nf.updateCallback(&route); err != nil {
				return err
			}
		}
	}
	return nil
}