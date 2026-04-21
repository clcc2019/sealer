package kubeadm

import (
	"fmt"

	versionUtils "github.com/sealerio/sealer/utils/version"
)

const (
	V1220 = "v1.22.0"
	V1310 = "v1.31.0"

	KubeadmV1beta3 = "kubeadm.k8s.io/v1beta3"
	KubeadmV1beta4 = "kubeadm.k8s.io/v1beta4"
)

func getKubeadmAPIVersion(kubernetesVersion string) (string, error) {
	kv := versionUtils.Version(kubernetesVersion)

	greaterOrEqualTo122, err := kv.GreaterThan(V1220)
	if err != nil {
		return "", fmt.Errorf("compare kubernetes version %s with %s: %w", kubernetesVersion, V1220, err)
	}

	greaterOrEqualTo131, err := kv.GreaterThan(V1310)
	if err != nil {
		return "", fmt.Errorf("compare kubernetes version %s with %s: %w", kubernetesVersion, V1310, err)
	}

	switch {
	case greaterOrEqualTo131:
		return KubeadmV1beta4, nil
	case greaterOrEqualTo122:
		return KubeadmV1beta3, nil
	default:
		return "", fmt.Errorf("kubernetes version %s is unsupported by this build, minimum supported version is %s", kubernetesVersion, V1220)
	}
}
