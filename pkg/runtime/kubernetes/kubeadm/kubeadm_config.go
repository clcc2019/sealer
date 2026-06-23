// Copyright © 2021 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubeadm

import (
	"fmt"
	"io"
	"net"
	"strings"

	kubeadmv1beta4 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta4"

	"github.com/sealerio/sealer/utils"
	strUtils "github.com/sealerio/sealer/utils/strings"

	"github.com/imdario/mergo"
	"k8s.io/kube-proxy/config/v1alpha1"
	"k8s.io/kubelet/config/v1beta1"
)

// Read config from https://github.com/sealerio/sealer/blob/main/docs/design/clusterfile-v2.md and overwrite default kubeadm.yaml
// Use github.com/imdario/mergo to merge kubeadm config in Clusterfile and the default kubeadm config
// Using a config filter to handle some edge cases

// nolint
type KubeadmConfig struct {
	kubeadmv1beta4.InitConfiguration
	kubeadmv1beta4.ClusterConfiguration
	v1alpha1.KubeProxyConfiguration
	v1beta1.KubeletConfiguration
	kubeadmv1beta4.JoinConfiguration
}

const (
	EtcdServers = "etcd-servers"
)

// LoadFromClusterfile :Load KubeadmConfig from Clusterfile.
// If it has `KubeadmConfig` in Clusterfile, load every field to each configuration.
// If Kubeadm raw config in Clusterfile, just load it.
func (k *KubeadmConfig) LoadFromClusterfile(kubeadmConfig KubeadmConfig) error {
	k.APIServer.CertSANs = strUtils.RemoveDuplicate(append(k.APIServer.CertSANs, kubeadmConfig.APIServer.CertSANs...))

	return mergeKubeadmConfig(k, kubeadmConfig)
}

// Merge Using github.com/imdario/mergo to merge KubeadmConfig to the ClusterImage default kubeadm config, overwrite some field.
// if defaultKubeadmConfig file not exist, use default raw kubeadm config to merge k.KubeConfigSpec empty value
func (k *KubeadmConfig) Merge(kubeadmYamlPath string, decode func(arg string, kind string) (interface{}, error)) error {
	newConfig, err := LoadKubeadmConfigs(kubeadmYamlPath, decode)
	if err != nil {
		return fmt.Errorf("failed to found kubeadm config from %s: %v", kubeadmYamlPath, err)
	}
	k.APIServer.CertSANs = strUtils.RemoveDuplicate(append(k.APIServer.CertSANs, newConfig.APIServer.CertSANs...))

	return mergeKubeadmConfig(k, newConfig)
}

func mergeKubeadmConfig(dst *KubeadmConfig, src KubeadmConfig) error {
	mergeExtraArgs(dst, src)

	return mergo.Merge(dst, src)
}

func mergeExtraArgs(dst *KubeadmConfig, src KubeadmConfig) {
	dst.ClusterConfiguration.APIServer.ExtraArgs = mergeArgsByName(dst.ClusterConfiguration.APIServer.ExtraArgs, src.ClusterConfiguration.APIServer.ExtraArgs)
	dst.ClusterConfiguration.ControllerManager.ExtraArgs = mergeArgsByName(dst.ClusterConfiguration.ControllerManager.ExtraArgs, src.ClusterConfiguration.ControllerManager.ExtraArgs)
	dst.ClusterConfiguration.Scheduler.ExtraArgs = mergeArgsByName(dst.ClusterConfiguration.Scheduler.ExtraArgs, src.ClusterConfiguration.Scheduler.ExtraArgs)
	dst.InitConfiguration.NodeRegistration.KubeletExtraArgs = mergeArgsByName(dst.InitConfiguration.NodeRegistration.KubeletExtraArgs, src.InitConfiguration.NodeRegistration.KubeletExtraArgs)
	dst.JoinConfiguration.NodeRegistration.KubeletExtraArgs = mergeArgsByName(dst.JoinConfiguration.NodeRegistration.KubeletExtraArgs, src.JoinConfiguration.NodeRegistration.KubeletExtraArgs)

	if dst.ClusterConfiguration.Etcd.Local != nil && src.ClusterConfiguration.Etcd.Local != nil {
		dst.ClusterConfiguration.Etcd.Local.ExtraArgs = mergeArgsByName(dst.ClusterConfiguration.Etcd.Local.ExtraArgs, src.ClusterConfiguration.Etcd.Local.ExtraArgs)
	}
}

func mergeArgsByName(dst, src []kubeadmv1beta4.Arg) []kubeadmv1beta4.Arg {
	if len(dst) == 0 {
		return append([]kubeadmv1beta4.Arg(nil), src...)
	}

	existing := make(map[string]struct{}, len(dst))
	for _, arg := range dst {
		existing[arg.Name] = struct{}{}
	}

	for _, arg := range src {
		if _, ok := existing[arg.Name]; ok {
			continue
		}
		dst = append(dst, arg)
	}

	return dst
}

func (k *KubeadmConfig) GetCertSANS() []string {
	return k.ClusterConfiguration.APIServer.CertSANs
}

func (k *KubeadmConfig) GetDNSDomain() string {
	return k.ClusterConfiguration.Networking.DNSDomain
}

func (k *KubeadmConfig) GetSvcCIDR() string {
	return k.ClusterConfiguration.Networking.ServiceSubnet
}

func LoadKubeadmConfigs(arg string, decode func(arg string, kind string) (interface{}, error)) (KubeadmConfig, error) {
	kubeadmConfig := KubeadmConfig{}
	initConfig, err := decode(arg, InitConfiguration)
	if err != nil && err != io.EOF {
		return kubeadmConfig, err
	} else if initConfig != nil {
		kubeadmConfig.InitConfiguration, err = NormalizeInitConfiguration(initConfig)
		if err != nil {
			return kubeadmConfig, err
		}
	}
	clusterConfig, err := decode(arg, ClusterConfiguration)
	if err != nil && err != io.EOF {
		return kubeadmConfig, err
	} else if clusterConfig != nil {
		kubeadmConfig.ClusterConfiguration, err = NormalizeClusterConfiguration(clusterConfig)
		if err != nil {
			return kubeadmConfig, err
		}
	}
	kubeProxyConfig, err := decode(arg, KubeProxyConfiguration)
	if err != nil && err != io.EOF {
		return kubeadmConfig, err
	} else if kubeProxyConfig != nil {
		kubeadmConfig.KubeProxyConfiguration = *kubeProxyConfig.(*v1alpha1.KubeProxyConfiguration)
	}
	kubeletConfig, err := decode(arg, KubeletConfiguration)
	if err != nil && err != io.EOF {
		return kubeadmConfig, err
	} else if kubeletConfig != nil {
		kubeadmConfig.KubeletConfiguration = *kubeletConfig.(*v1beta1.KubeletConfiguration)
	}
	joinConfig, err := decode(arg, JoinConfiguration)
	if err != nil && err != io.EOF {
		return kubeadmConfig, err
	} else if joinConfig != nil {
		kubeadmConfig.JoinConfiguration, err = NormalizeJoinConfiguration(joinConfig)
		if err != nil {
			return kubeadmConfig, err
		}
	}
	return kubeadmConfig, nil
}

func getEtcdEndpointsWithHTTPSPrefix(masters []net.IP) string {
	var tmpSlice []string
	for _, ip := range masters {
		tmpSlice = append(tmpSlice, fmt.Sprintf("https://%s", net.JoinHostPort(ip.String(), "2379")))
	}

	return strings.Join(tmpSlice, ",")
}

func setArg(args []kubeadmv1beta4.Arg, name, value string) []kubeadmv1beta4.Arg {
	for i := range args {
		if args[i].Name == name {
			args[i].Value = value
			return args
		}
	}

	return append(args, kubeadmv1beta4.Arg{Name: name, Value: value})
}

func NewKubeadmConfig(fromClusterFile KubeadmConfig, fromFile string, masters []net.IP, apiServerDomain,
	cgroupDriver string, imageRepo string, apiServerVIP net.IP, extraSANs []string) (KubeadmConfig, error) {
	conf := KubeadmConfig{}

	if len(masters) == 0 {
		return conf, fmt.Errorf("at least one master is required to generate kubeadm config")
	}

	if err := conf.LoadFromClusterfile(fromClusterFile); err != nil {
		return conf, fmt.Errorf("failed to load kubeadm config from clusterfile: %v", err)
	}
	// TODO handle the kubeadm config, like kubeproxy config
	//The configuration set here does not require merge

	conf.InitConfiguration.LocalAPIEndpoint.AdvertiseAddress = masters[0].String()
	conf.ControlPlaneEndpoint = net.JoinHostPort(apiServerDomain, "6443")
	conf.APIServer.ExtraArgs = setArg(conf.APIServer.ExtraArgs, EtcdServers, getEtcdEndpointsWithHTTPSPrefix(masters))
	conf.IPVS.ExcludeCIDRs = append(conf.KubeProxyConfiguration.IPVS.ExcludeCIDRs, fmt.Sprintf("%s/32", apiServerVIP))
	conf.KubeletConfiguration.CgroupDriver = cgroupDriver
	conf.ClusterConfiguration.APIServer.CertSANs = []string{"127.0.0.1", apiServerDomain, apiServerVIP.String()}
	conf.ClusterConfiguration.APIServer.CertSANs = append(conf.ClusterConfiguration.APIServer.CertSANs, extraSANs...)
	for _, m := range masters {
		conf.ClusterConfiguration.APIServer.CertSANs = append(conf.ClusterConfiguration.APIServer.CertSANs, m.String())
	}

	if err := conf.Merge(fromFile, utils.DecodeCRDFromFile); err != nil {
		return conf, err
	}

	if err := conf.Merge(DefaultKubeadmConfig, utils.DecodeCRDFromString); err != nil {
		return conf, err
	}

	if err := conf.setKubeadmAPIVersion(); err != nil {
		return conf, err
	}

	if conf.ClusterConfiguration.Networking.DNSDomain == "" {
		conf.ClusterConfiguration.Networking.DNSDomain = "cluster.local"
	}
	if conf.JoinConfiguration.Discovery.BootstrapToken == nil {
		conf.JoinConfiguration.Discovery.BootstrapToken = &kubeadmv1beta4.BootstrapTokenDiscovery{}
	}

	// set cluster image repo,kubeadm will pull container image from this registry.
	if conf.ClusterConfiguration.ImageRepository == "" {
		conf.ClusterConfiguration.ImageRepository = imageRepo
	}
	if conf.ClusterConfiguration.DNS.ImageMeta.ImageRepository == "" {
		conf.ClusterConfiguration.DNS.ImageMeta.ImageRepository = fmt.Sprintf("%s/%s", imageRepo, "coredns")
	}

	return conf, nil
}
