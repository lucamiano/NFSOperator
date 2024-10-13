/*
Copyright 2018 The Kubernetes Authors.

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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io

// podAnnotator annotates Pods
type podAnnotator struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (a *podAnnotator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	err := a.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// check if we are in the correct namespace, otherwise let the request succeed
	if pod.Namespace != "home" {
		return admission.Allowed("Not the target namespace")
	}

	// get service account from request
	username := req.UserInfo.Username
	if !strings.HasPrefix(username, "system:serviceaccount:") {
		return admission.Denied("Request is not from a ServiceAccount")
	}
	saParts := strings.Split(username, ":")
	if len(saParts) != 4 {
		return admission.Denied("Invalid ServiceAccount format in request")
	}
	serviceAccount := saParts[3]

	// get serviceaccount-uid configmap
	configMap := &corev1.ConfigMap{}
	err = a.Client.Get(ctx, types.NamespacedName{
		Name:      "uid-configmap",
		Namespace: "home",
	}, configMap)
	if err != nil {
		if errors.IsNotFound(err) {
			return admission.Denied("ConfigMap not found")
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// extract uid from serviceaccount
	uid, found := configMap.Data[serviceAccount]
	if found {
		// Set runAsUser
		pod.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser: func() *int64 {
				uidValue, _ := strconv.ParseInt(uid, 10, 64)
				return &uidValue
			}(),
		}
	}

	// rewrite Pod spec
	marshaledPod, err := json.Marshal(pod)

	// if pod.Annotations == nil {
	// 	pod.Annotations = map[string]string{}
	// }
	// pod.Annotations["add-nfs-webhook"] = "foo"
	// if err != nil {
	// 	return admission.Errored(http.StatusInternalServerError, err)
	// }
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}
