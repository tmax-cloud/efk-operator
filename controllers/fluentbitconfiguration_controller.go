/*


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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	configv1alpha1 "github.com/tmax-cloud/efk-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	serviceTemplate = `[SERVICE]
    Flush         5
    Daemon        off
    Log_Level     info
`
	inputTemplate = `[INPUT]
    Name  tail
    Path  /shared/[PATTERN]
    Tag   [TAG]
`
	parserTemplate = `[PARSER]
    Name    [PARSER_NAME]
    Format  regex
    Regex   [REGEX]
`
	filterTemplate = `[FILTER]
    Name          parser
    Key_name      log
    Reserve_Data  On
    Preserve_Key  On
    Parser        [PARSER_NAME]
    Match         [TAG]
`
	outputTemplate = `[OUTPUT]
    Name             es
    Host             [ES_HOST]
    Port             [ES_PORT]
    Logstash_Format  True
    Logstash_Prefix  [INDEX_NAME]
    Match            [TAG]
`
	parserFileConfig = `    Parsers_File  /fluent-bit/etc/user-parser.conf
`
)

var (
	ElasticsearchHost string
	ElasticsearchPort string
)

// FluentBitConfigurationReconciler reconciles a FluentBitConfiguration object
type FluentBitConfigurationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=config.tmax.io,resources=fluentbitconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.tmax.io,resources=fluentbitconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch

func (r *FluentBitConfigurationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := r.Log.WithValues("fluentbitconfiguration", req.NamespacedName)

	// your logic here

	fbc := configv1alpha1.FluentBitConfiguration{}
	err := r.Get(context.TODO(), req.NamespacedName, &fbc)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("FluentBitConfiguration resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get FluentBitConfiguration")
		return ctrl.Result{}, err
	}

	logRootPath := getLogRootPath(fbc.Spec.InputPlugins)

	// Check if the configmap already exists, if not create a new one
	found := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: fbc.Name, Namespace: fbc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new configmap
		cfgmap := r.configMapForLogCrd(&fbc, logRootPath)
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", cfgmap.Namespace, "ConfigMap.Name", cfgmap.Name)
		err = r.Create(context.TODO(), cfgmap)
		if err != nil {
			log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", cfgmap.Namespace, "ConfigMap.Name", cfgmap.Name)
			return ctrl.Result{}, err
		}
		err = r.Get(context.TODO(), types.NamespacedName{Name: fbc.Name, Namespace: fbc.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Waiting for configmap", "ConfigMap.Namespace", cfgmap.Namespace, "ConfigMap.Name", cfgmap.Name)
			return ctrl.Result{Requeue: true}, nil
		}
	} else if err != nil {
		fbc.Status.Phase = "error"
		err = r.Status().Update(context.TODO(), &fbc)
		if err != nil {
			log.Error(err, "Failed to update FluentBitConfiguration status")
			return ctrl.Result{}, err
		}
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	} else {
		// Update the FluentBitConfiguration status to created
		fbc.Status.Phase = "created"
		fbc.Status.LogRootPath = logRootPath
		err = r.Status().Update(context.TODO(), &fbc)
		if err != nil {
			log.Error(err, "Failed to update FluentBitConfiguration status")
			return ctrl.Result{}, err
		}
	}

	logRootPath = getLogRootPath(fbc.Spec.InputPlugins)
	fd := strings.Compare(found.Data["fluent-bit.conf"], buildFluentBitConfigFile(&fbc, logRootPath))
	ud := strings.Compare(found.Data["user-parser.conf"], buildParserConfigFile(fbc.Spec.FilterPlugins))
	if fd != 0 || ud != 0 {
		patch := client.MergeFrom(found.DeepCopy())
		found.Data["fluent-bit.conf"] = buildFluentBitConfigFile(&fbc, logRootPath)
		found.Data["user-parser.conf"] = buildParserConfigFile(fbc.Spec.FilterPlugins)
		err = r.Patch(context.TODO(), found, patch)
		if err != nil {
			log.Error(err, "Failed to patch.")
			return ctrl.Result{}, err
		}

		fbc.Status.LogRootPath = logRootPath
		err = r.Status().Update(context.TODO(), &fbc)
		if err != nil {
			log.Error(err, "Failed to update FluentBitConfiguration status")
			return ctrl.Result{}, err
		}

		deployList := &appsv1.DeploymentList{}
		if err = r.List(context.TODO(), deployList, client.InNamespace(fbc.Namespace), client.MatchingLabels{"tmax.io/log-collector-configuration": fbc.Name}); err != nil {
			log.Error(err, "Failed to list deployment")
			return ctrl.Result{}, err
		}

		for _, deploy := range deployList.Items {
			// current RS를 찾아서 저장해 놓았다가 (rs 중에서 spec.replica != 0이 아닌 rs를 저장)
			// PATCH 이후 삭제
			var rs *appsv1.ReplicaSet
			if rs, err = r.getCurrentReplicaSet(&deploy); err != nil {

			}

			patch := client.MergeFrom(deploy.DeepCopy())
			if deploy.Spec.Template.Annotations == nil {
				deploy.Spec.Template.Annotations = map[string]string{}
			}
			deploy.Spec.Template.Annotations["tmax.io/restartedAt"] = time.Now().String()
			err := r.Patch(context.TODO(), &deploy, patch)
			if err != nil {
				log.Error(err, "Failed to patch deployment (rolling update).")
				return ctrl.Result{}, err
			}

			if err = r.Delete(context.TODO(), rs); err != nil {
				log.Error(err, "Failed to delete old rs (rolling update).")
				return ctrl.Result{}, err
			}
		}

		dsList := &appsv1.DaemonSetList{}
		if err = r.List(context.TODO(), dsList, client.InNamespace(fbc.Namespace), client.MatchingLabels{"tmax.io/log-collector-configuration": fbc.Name}); err != nil {
			log.Error(err, "Failed to list daemonset")
			return ctrl.Result{}, err
		}

		for _, ds := range dsList.Items {
			patch := client.MergeFrom(ds.DeepCopy())
			if ds.Spec.Template.Annotations == nil {
				ds.Spec.Template.Annotations = map[string]string{}
			}
			ds.Spec.Template.Annotations["tmax.io/restartedAt"] = time.Now().String()
			err := r.Patch(context.TODO(), &ds, patch)
			if err != nil {
				log.Error(err, "Failed to patch daemonset (rolling update).")
				return ctrl.Result{}, err
			}
		}

		stsList := &appsv1.StatefulSetList{}
		if err = r.List(context.TODO(), stsList, client.InNamespace(fbc.Namespace), client.MatchingLabels{"tmax.io/log-collector-configuration": fbc.Name}); err != nil {
			log.Error(err, "Failed to list statefulset")
			return ctrl.Result{}, err
		}

		for _, sts := range stsList.Items {
			patch := client.MergeFrom(sts.DeepCopy())
			if sts.Spec.Template.Annotations == nil {
				sts.Spec.Template.Annotations = map[string]string{}
			}
			sts.Spec.Template.Annotations["tmax.io/restartedAt"] = time.Now().String()
			err := r.Patch(context.TODO(), &sts, patch)
			if err != nil {
				log.Error(err, "Failed to patch statefulset (rolling update).")
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

//
func (r *FluentBitConfigurationReconciler) getCurrentReplicaSet(deployment *appsv1.Deployment) (*appsv1.ReplicaSet, error) {
	namespace := deployment.Namespace
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	fmt.Println(selector.String())

	rsList := &appsv1.ReplicaSetList{}
	if err = r.List(context.TODO(), rsList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		log.Error(err, "Failed to list statefulset")
		return nil, err
	}

	b, _ := json.Marshal(rsList)
	fmt.Println(string(b))
	var rss []*appsv1.ReplicaSet
	for i := range rsList.Items {
		rss = append(rss, &rsList.Items[i])
	}

	// // Only include those whose ControllerRef matches the Deployment.
	owned := make([]*appsv1.ReplicaSet, 0, len(rss))
	for _, rs := range rss {
		if metav1.IsControlledBy(rs, deployment) {
			owned = append(owned, rs)
		}
	}
	b1, _ := json.Marshal(owned)
	fmt.Println(string(b1))

	// replicas == 0?
	var ret *appsv1.ReplicaSet
	for _, rs := range owned {
		if *rs.Spec.Replicas != 0 {
			ret = rs
		}
	}
	b2, _ := json.Marshal(ret)
	fmt.Println(string(b2))
	return ret, err
}

// configMapForLogCrd returns a fbc configmap object
func (r *FluentBitConfigurationReconciler) configMapForLogCrd(fbc *configv1alpha1.FluentBitConfiguration, logRootPath string) *corev1.ConfigMap {
	cfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fbc.Name,
			Namespace: fbc.Namespace,
		},
		Data: map[string]string{
			"fluent-bit.conf":  buildFluentBitConfigFile(fbc, logRootPath),
			"user-parser.conf": buildParserConfigFile(fbc.Spec.FilterPlugins),
		},
	}
	// Set FluentBitConfiguration instance as the owner and controller
	ctrl.SetControllerReference(fbc, cfgMap, r.Scheme)
	return cfgMap
}

// Build fluent-bit configMap using spec
func buildFluentBitConfigFile(fbc *configv1alpha1.FluentBitConfiguration, logRootPath string) string {
	var configs []string

	configs = append(configs, replaceServicePlugin(fbc.Spec.FilterPlugins))
	configs = append(configs, replaceInputPlugin(fbc.Spec.InputPlugins, logRootPath))
	configs = append(configs, replaceFilterPlugin(fbc.Spec.FilterPlugins))
	configs = append(configs, replaceOutputPlugin(fbc.Spec.OutputPlugins))
	result := strings.Join(configs, "\n")
	return result
}

func replaceServicePlugin(filterPlugins []configv1alpha1.FilterPlugin) string {
	var services []string
	if filterPlugins == nil {
		return serviceTemplate
	} else {
		services = append(services, serviceTemplate)
		services = append(services, parserFileConfig)
	}
	result := strings.Join(services, "")
	return result
}

func replaceInputPlugin(inputPlugins []configv1alpha1.InputPlugin, logRootPath string) string {

	var inputs []string
	for _, inputPlugin := range inputPlugins {
		pathElem := strings.Split(strings.Split(inputPlugin.Path, logRootPath)[1], string(os.PathSeparator))
		//TODO seperator 없는 STRING 예외 처리
		middlePath := pathElem[1 : len(pathElem)-1]
		middlePath = append(middlePath, inputPlugin.Pattern)
		tmp := strings.Replace(inputTemplate, "[PATTERN]", strings.Join(middlePath, "/"), -1)
		tmp = strings.Replace(tmp, "[TAG]", inputPlugin.Tag, -1)
		inputs = append(inputs, tmp)
	}
	result := strings.Join(inputs, "\n")
	return result
}

func replaceFilterPlugin(filterPlugins []configv1alpha1.FilterPlugin) string {
	var filters []string
	for _, filterPlugin := range filterPlugins {
		tmp := strings.Replace(filterTemplate, "[PARSER_NAME]", filterPlugin.ParserName, -1)
		tmp = strings.Replace(tmp, "[TAG]", filterPlugin.Tag, -1)
		filters = append(filters, tmp)
	}
	result := strings.Join(filters, "\n")
	return result
}

func replaceOutputPlugin(outputPlugins []configv1alpha1.OutputPlugin) string {
	var outputs []string
	for _, outputPlugin := range outputPlugins {
		tmp := strings.Replace(outputTemplate, "[ES_HOST]", ElasticsearchHost, -1)
		tmp = strings.Replace(tmp, "[ES_PORT]", ElasticsearchPort, -1)
		tmp = strings.Replace(tmp, "[INDEX_NAME]", outputPlugin.IndexName, -1)
		tmp = strings.Replace(tmp, "[TAG]", outputPlugin.Tag, -1)
		outputs = append(outputs, tmp)
	}
	result := strings.Join(outputs, "\n")
	return result
}

func buildParserConfigFile(filterPlugins []configv1alpha1.FilterPlugin) string {
	var parsers []string
	for _, filterPlugin := range filterPlugins {
		tmp := strings.Replace(parserTemplate, "[PARSER_NAME]", filterPlugin.ParserName, -1)
		tmp = strings.Replace(tmp, "[REGEX]", filterPlugin.Regex, -1)
		parsers = append(parsers, tmp)
	}
	result := strings.Join(parsers, "\n")
	return result
}

func getLogRootPath(inputPlugins []configv1alpha1.InputPlugin) string {
	var pathList []string
	for _, inputPlugin := range inputPlugins {
		pathList = append(pathList, inputPlugin.Path)
	}
	return commonPrefix(os.PathSeparator, pathList)
}

func commonPrefix(sep byte, paths []string) string {
	switch len(paths) {
	case 0:
		return ""
	case 1:
		pathElem := strings.Split(paths[0], string(os.PathSeparator))
		return strings.Join(pathElem[0:len(pathElem)-1], string(os.PathSeparator))
	}

	c := []byte(path.Clean(paths[0]))
	c = append(c, sep)
	for _, v := range paths[1:] {
		v = path.Clean(v) + string(sep)
		if len(v) < len(c) {
			c = c[:len(v)]
		}
		for i := 0; i < len(c); i++ {
			if v[i] != c[i] {
				c = c[:i]
				break
			}
		}
	}

	for i := len(c) - 1; i >= 0; i-- {
		if c[i] == sep {
			c = c[:i]
			break
		}
	}

	return string(c)
}

func (r *FluentBitConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// return ctrl.NewControllerManagedBy(mgr).
	// 	For(&configv1alpha1.FluentBitConfiguration{}).
	// 	Complete(r)

	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&configv1alpha1.FluentBitConfiguration{}).
		Build(r)

	if err != nil {
		return err
	}

	return controller.Watch(
		&source.Kind{Type: &corev1.ConfigMap{}},
		&handler.EnqueueRequestForOwner{
			OwnerType: &configv1alpha1.FluentBitConfiguration{}, IsController: true,
		},
		predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
	)

}
