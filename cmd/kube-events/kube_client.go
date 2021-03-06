package main

import (
	"strings"

	"github.com/containerum/kube-events/pkg/transform"

	"github.com/containerum/kube-events/pkg/informerwatch"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Kube struct {
	*kubernetes.Clientset
	config *rest.Config
}

type Watchers struct {
	ResourceQuotas watch.Interface //Namespaces
	Deployments    watch.Interface
	Events         watch.Interface //Pods
	Services       watch.Interface
	Ingresses      watch.Interface
	Secrets        watch.Interface
	ConfigMaps     watch.Interface
	PVCs           watch.Interface //Volumes
}

func (k *Kube) WatchSupportedResources() Watchers {
	informerFactory := informers.NewSharedInformerFactory(k.Clientset, 0)

	rqWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().ResourceQuotas().Informer())
	deplWatch := informerwatch.NewInformerWatch(informerFactory.Apps().V1().Deployments().Informer())
	eventWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().Events().Informer())
	serviceWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().Services().Informer())
	ingressWatch := informerwatch.NewInformerWatch(informerFactory.Extensions().V1beta1().Ingresses().Informer())
	pvcWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().PersistentVolumeClaims().Informer())
	secretWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().Secrets().Informer())
	cmWatch := informerwatch.NewInformerWatch(informerFactory.Core().V1().ConfigMaps().Informer())

	logrus.Infof("Watching for: %s", strings.Join([]string{
		"ResourceQuota",
		"Deployment",
		"Event",
		"Service",
		"Ingress",
		"PersistentVolumeClaim",
		"Secret",
		"ConfigMap",
	}, ","))

	return Watchers{
		ResourceQuotas: transform.NewFilteredWatch(rqWatch, ResourceQuotaFilter, ErrorFilter),
		Deployments:    transform.NewFilteredWatch(deplWatch, NewDeployFilter().Filter, ErrorFilter),
		Events:         transform.NewFilteredWatch(eventWatch, EventsFilter, ErrorFilter),
		Services:       transform.NewFilteredWatch(serviceWatch, ErrorFilter),
		Ingresses:      transform.NewFilteredWatch(ingressWatch, ErrorFilter),
		PVCs:           transform.NewFilteredWatch(pvcWatch, PVCFilter, ErrorFilter),
		Secrets:        transform.NewFilteredWatch(secretWatch, ErrorFilter),
		ConfigMaps:     transform.NewFilteredWatch(cmWatch, ErrorFilter),
	}
}
