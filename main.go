package main

import (
	"encoding/json"
	"time"

	"github.com/caarlos0/env"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/kubernetes-sigs/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/kyma-project/helm-broker/pkg/apis/addons/v1alpha1"
	"github.com/kyma-project/helm-broker/pkg/client/clientset/versioned"
	"github.com/prometheus/common/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type settings struct {
	Namespace string `env:"NAMESPACE"`
	AddonPath string `env:"ADDON_PATH"`
	Username  string `env:"SM_USER"`
	Password  string `env:"SM_PASSWORD"`
	URL       string `env:"SM_URL"`
}

const (
	secretName           = "service-manager-credentials"
	clusterAddonConfName = "broker-proxy-k8s-addon"
	serviceInstanceName  = "service-broker-proxy-k8s"
	clusterServiceClass  = "service-broker-proxy-k8s"
	clusterServicePlan   = "default"
)

func main() {
	log.Info("Start install process, version: 0.1.3")

	stg := settings{}
	err := env.Parse(&stg)
	fatalOnError(err, "during parse env")

	k8sKubeconfig := config.GetConfigOrDie()

	clientk8s, err := kubernetes.NewForConfig(k8sKubeconfig)
	fatalOnError(err, "during get k8s client")

	addonClient, err := versioned.NewForConfig(k8sKubeconfig)
	fatalOnError(err, "during get addons configuration client")

	scClient, err := clientset.NewForConfig(k8sKubeconfig)
	fatalOnError(err, "during get service catalog client")

	log.Info("Create secret")
	err = createSecret(clientk8s, stg)
	fatalOnError(err, "during creating secret")

	log.Info("Create ClusterAddonConfiguration")
	err = createClusterAddonConfiguration(addonClient, stg)
	fatalOnError(err, "during creating ClusterAddonConfiguration")

	log.Info("Create ServiceInstance")
	err = createServiceInstance(*scClient, stg)
	fatalOnError(err, "during creating ServiceInstance")

	log.Info("Connection with ServiceManager is ready")
}

func createSecret(client *kubernetes.Clientset, s settings) error {
	_, err := client.CoreV1().Secrets(s.Namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: s.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte(s.Username),
			"password": []byte(s.Password),
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func createClusterAddonConfiguration(client *versioned.Clientset, s settings) error {
	_, err := client.AddonsV1alpha1().ClusterAddonsConfigurations().Create(&v1alpha1.ClusterAddonsConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterAddonConfName,
		},
		Spec: v1alpha1.ClusterAddonsConfigurationSpec{
			CommonAddonsConfigurationSpec: v1alpha1.CommonAddonsConfigurationSpec{
				Repositories: []v1alpha1.SpecRepository{{
					URL: s.AddonPath,
				},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	err = wait.Poll(5*time.Second, 120*time.Second, func() (done bool, err error) {
		cac, err := client.AddonsV1alpha1().ClusterAddonsConfigurations().Get(clusterAddonConfName, metav1.GetOptions{})
		if err != nil {
			log.Errorf("error during get ClusterAddonsConfiguration: %s", err)
			return false, nil
		}
		for _, repository := range cac.Status.Repositories {
			if repository.Status == v1alpha1.RepositoryStatusReady {
				return true, nil
			}
		}

		log.Info("ClusterAddonsConfiguration is not ready, retry...")
		return false, nil
	})

	return nil
}

func createServiceInstance(client clientset.Clientset, s settings) error {
	parameters, err := convertParametersIntoRawExtension(map[string]interface{}{
		"config": map[string]interface{}{
			"sm": map[string]interface{}{
				"url": s.URL,
			},
		},
		"secretName": secretName,
	})
	if err != nil {
		return err
	}

	_, err = client.ServicecatalogV1beta1().ServiceInstances(s.Namespace).Create(&v1beta1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceInstanceName,
			Namespace: s.Namespace,
		},
		Spec: v1beta1.ServiceInstanceSpec{
			PlanReference: v1beta1.PlanReference{
				ClusterServiceClassExternalName: clusterServiceClass,
				ClusterServicePlanExternalName:  clusterServicePlan,
			},
			Parameters: parameters,
		},
	})
	if err != nil {
		return err
	}

	err = wait.Poll(5*time.Second, 60*time.Second, func() (done bool, err error) {
		inst, err := client.ServicecatalogV1beta1().ServiceInstances(s.Namespace).Get(serviceInstanceName, metav1.GetOptions{})
		if err != nil {
			log.Errorf("error during get ServiceInstance: %s", err)
			return false, nil
		}
		for _, cond := range inst.Status.Conditions {
			if cond.Type == v1beta1.ServiceInstanceConditionReady && cond.Status == v1beta1.ConditionTrue {
				return true, nil
			}
		}
		log.Info("ServiceInstance is not ready, retry...")
		return false, nil
	})

	return nil
}

func convertParametersIntoRawExtension(parameters map[string]interface{}) (*runtime.RawExtension, error) {
	marshalledParams, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: marshalledParams}, nil
}

func fatalOnError(err error, context string) {
	if err != nil {
		klog.Fatalf("%s: %v", context, err)
	}
}
