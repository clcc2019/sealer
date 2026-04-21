package kubeadm

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmv1beta3 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	kubeadmv1beta4 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta4"

	sealeryaml "github.com/sealerio/sealer/utils/yaml"
)

var scheme = runtime.NewScheme()

func init() {
	kubeadmscheme.AddToScheme(scheme)
}

func NormalizeInitConfiguration(config interface{}) (kubeadmv1beta4.InitConfiguration, error) {
	switch cfg := config.(type) {
	case *kubeadmv1beta4.InitConfiguration:
		return *cfg, nil
	case *kubeadmv1beta3.InitConfiguration:
		out := kubeadmv1beta4.InitConfiguration{}
		if err := scheme.Convert(cfg, &out, nil); err != nil {
			return out, fmt.Errorf("convert kubeadm init configuration to v1beta4: %w", err)
		}
		return out, nil
	default:
		return kubeadmv1beta4.InitConfiguration{}, fmt.Errorf("unsupported kubeadm init configuration type %T", config)
	}
}

func NormalizeClusterConfiguration(config interface{}) (kubeadmv1beta4.ClusterConfiguration, error) {
	switch cfg := config.(type) {
	case *kubeadmv1beta4.ClusterConfiguration:
		return *cfg, nil
	case *kubeadmv1beta3.ClusterConfiguration:
		out := kubeadmv1beta4.ClusterConfiguration{}
		if err := scheme.Convert(cfg, &out, nil); err != nil {
			return out, fmt.Errorf("convert kubeadm cluster configuration to v1beta4: %w", err)
		}
		return out, nil
	default:
		return kubeadmv1beta4.ClusterConfiguration{}, fmt.Errorf("unsupported kubeadm cluster configuration type %T", config)
	}
}

func NormalizeJoinConfiguration(config interface{}) (kubeadmv1beta4.JoinConfiguration, error) {
	switch cfg := config.(type) {
	case *kubeadmv1beta4.JoinConfiguration:
		return *cfg, nil
	case *kubeadmv1beta3.JoinConfiguration:
		out := kubeadmv1beta4.JoinConfiguration{}
		if err := scheme.Convert(cfg, &out, nil); err != nil {
			return out, fmt.Errorf("convert kubeadm join configuration to v1beta4: %w", err)
		}
		return out, nil
	default:
		return kubeadmv1beta4.JoinConfiguration{}, fmt.Errorf("unsupported kubeadm join configuration type %T", config)
	}
}

func (k *KubeadmConfig) setKubeadmAPIVersion() error {
	apiVersion, err := getKubeadmAPIVersion(k.KubernetesVersion)
	if err != nil {
		return err
	}

	k.InitConfiguration.APIVersion = apiVersion
	k.InitConfiguration.Kind = InitConfiguration
	k.ClusterConfiguration.APIVersion = apiVersion
	k.ClusterConfiguration.Kind = ClusterConfiguration
	k.JoinConfiguration.APIVersion = apiVersion
	k.JoinConfiguration.Kind = JoinConfiguration

	return nil
}

func (k KubeadmConfig) MarshalInitStack() ([]byte, error) {
	apiVersion, err := getKubeadmAPIVersion(k.KubernetesVersion)
	if err != nil {
		return nil, err
	}

	initConfig, clusterConfig, joinConfig, err := exportKubeadmConfigs(k, apiVersion)
	if err != nil {
		return nil, err
	}

	return sealeryaml.MarshalWithDelimiter(
		initConfig,
		clusterConfig,
		&k.KubeletConfiguration,
		&k.KubeProxyConfiguration,
		joinConfig,
	)
}

func (k KubeadmConfig) MarshalJoinStack() ([]byte, error) {
	apiVersion, err := getKubeadmAPIVersion(k.KubernetesVersion)
	if err != nil {
		return nil, err
	}

	_, _, joinConfig, err := exportKubeadmConfigs(k, apiVersion)
	if err != nil {
		return nil, err
	}

	return sealeryaml.MarshalWithDelimiter(joinConfig, &k.KubeletConfiguration)
}

func (k KubeadmConfig) ExportClusterfileObjects() ([]interface{}, error) {
	apiVersion, err := getKubeadmAPIVersion(k.KubernetesVersion)
	if err != nil {
		return nil, err
	}

	initConfig, clusterConfig, joinConfig, err := exportKubeadmConfigs(k, apiVersion)
	if err != nil {
		return nil, err
	}

	var configs []interface{}
	if k.InitConfiguration.Kind != "" {
		configs = append(configs, initConfig)
	}
	if k.JoinConfiguration.Kind != "" {
		configs = append(configs, joinConfig)
	}
	if k.ClusterConfiguration.Kind != "" {
		configs = append(configs, clusterConfig)
	}
	if k.KubeletConfiguration.Kind != "" {
		configs = append(configs, &k.KubeletConfiguration)
	}
	if k.KubeProxyConfiguration.Kind != "" {
		configs = append(configs, &k.KubeProxyConfiguration)
	}

	return configs, nil
}

func exportKubeadmConfigs(config KubeadmConfig, apiVersion string) (interface{}, interface{}, interface{}, error) {
	switch apiVersion {
	case KubeadmV1beta4:
		initConfig := config.InitConfiguration
		initConfig.APIVersion = apiVersion
		initConfig.Kind = InitConfiguration

		clusterConfig := config.ClusterConfiguration
		clusterConfig.APIVersion = apiVersion
		clusterConfig.Kind = ClusterConfiguration

		joinConfig := config.JoinConfiguration
		joinConfig.APIVersion = apiVersion
		joinConfig.Kind = JoinConfiguration

		return &initConfig, &clusterConfig, &joinConfig, nil
	case KubeadmV1beta3:
		initConfig := kubeadmv1beta3.InitConfiguration{}
		if err := scheme.Convert(&config.InitConfiguration, &initConfig, nil); err != nil {
			return nil, nil, nil, fmt.Errorf("convert kubeadm init configuration to v1beta3: %w", err)
		}
		initConfig.APIVersion = apiVersion
		initConfig.Kind = InitConfiguration

		clusterConfig := kubeadmv1beta3.ClusterConfiguration{}
		if err := scheme.Convert(&config.ClusterConfiguration, &clusterConfig, nil); err != nil {
			return nil, nil, nil, fmt.Errorf("convert kubeadm cluster configuration to v1beta3: %w", err)
		}
		clusterConfig.APIVersion = apiVersion
		clusterConfig.Kind = ClusterConfiguration

		joinConfig := kubeadmv1beta3.JoinConfiguration{}
		if err := scheme.Convert(&config.JoinConfiguration, &joinConfig, nil); err != nil {
			return nil, nil, nil, fmt.Errorf("convert kubeadm join configuration to v1beta3: %w", err)
		}
		joinConfig.APIVersion = apiVersion
		joinConfig.Kind = JoinConfiguration

		return &initConfig, &clusterConfig, &joinConfig, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported kubeadm api version %s", apiVersion)
	}
}
