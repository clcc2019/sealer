package kubeadm

import (
	"strings"
	"testing"
	"time"

	"github.com/sealerio/sealer/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1beta3 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta3"
	kubeadmv1beta4 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta4"
)

func TestGetKubeadmAPIVersion(t *testing.T) {
	tests := []struct {
		name              string
		kubernetesVersion string
		want              string
		wantErr           bool
	}{
		{
			name:              "v1.30 uses kubeadm v1beta3",
			kubernetesVersion: "v1.30.9",
			want:              KubeadmV1beta3,
		},
		{
			name:              "v1.31 uses kubeadm v1beta4",
			kubernetesVersion: "v1.31.0",
			want:              KubeadmV1beta4,
		},
		{
			name:              "v1.35 uses kubeadm v1beta4",
			kubernetesVersion: "v1.35.4",
			want:              KubeadmV1beta4,
		},
		{
			name:              "versions older than v1.22 are unsupported",
			kubernetesVersion: "v1.21.14",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKubeadmAPIVersion(tt.kubernetesVersion)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExportKubeadmConfigsConvertsExtraArgsForTargetAPIVersion(t *testing.T) {
	config := KubeadmConfig{}
	config.ClusterConfiguration.APIServer.ExtraArgs = []kubeadmv1beta4.Arg{
		{Name: "audit-log-format", Value: "json"},
	}

	_, clusterV1beta3, _, err := exportKubeadmConfigs(config, KubeadmV1beta3)
	if err != nil {
		t.Fatalf("export v1beta3: %v", err)
	}
	gotV1beta3 := clusterV1beta3.(*kubeadmv1beta3.ClusterConfiguration)
	if gotV1beta3.APIVersion != KubeadmV1beta3 {
		t.Fatalf("got v1beta3 apiVersion %q, want %q", gotV1beta3.APIVersion, KubeadmV1beta3)
	}
	if gotV1beta3.APIServer.ExtraArgs["audit-log-format"] != "json" {
		t.Fatalf("v1beta3 extraArgs was not converted to a map: %#v", gotV1beta3.APIServer.ExtraArgs)
	}

	_, clusterV1beta4, _, err := exportKubeadmConfigs(config, KubeadmV1beta4)
	if err != nil {
		t.Fatalf("export v1beta4: %v", err)
	}
	gotV1beta4 := clusterV1beta4.(*kubeadmv1beta4.ClusterConfiguration)
	if gotV1beta4.APIVersion != KubeadmV1beta4 {
		t.Fatalf("got v1beta4 apiVersion %q, want %q", gotV1beta4.APIVersion, KubeadmV1beta4)
	}
	if len(gotV1beta4.APIServer.ExtraArgs) != 1 || gotV1beta4.APIServer.ExtraArgs[0].Name != "audit-log-format" {
		t.Fatalf("v1beta4 extraArgs was not kept as name/value args: %#v", gotV1beta4.APIServer.ExtraArgs)
	}
}

func TestExportKubeadmConfigsDefaultsJoinDiscoveryForConversion(t *testing.T) {
	config := KubeadmConfig{}
	config.JoinConfiguration.Discovery.TLSBootstrapToken = "abcdef.0123456789abcdef"

	_, _, joinV1beta3, err := exportKubeadmConfigs(config, KubeadmV1beta3)
	if err != nil {
		t.Fatalf("export v1beta3: %v", err)
	}
	gotV1beta3 := joinV1beta3.(*kubeadmv1beta3.JoinConfiguration)
	if gotV1beta3.Discovery.BootstrapToken == nil {
		t.Fatal("expected v1beta3 bootstrap token discovery to be initialized")
	}
	if gotV1beta3.Discovery.Timeout == nil || gotV1beta3.Discovery.Timeout.Duration != 5*time.Minute {
		t.Fatalf("unexpected v1beta3 discovery timeout: %#v", gotV1beta3.Discovery.Timeout)
	}
	if gotV1beta3.Discovery.TLSBootstrapToken != "abcdef.0123456789abcdef" {
		t.Fatalf("unexpected v1beta3 tls bootstrap token %q", gotV1beta3.Discovery.TLSBootstrapToken)
	}

	_, _, joinV1beta4, err := exportKubeadmConfigs(config, KubeadmV1beta4)
	if err != nil {
		t.Fatalf("export v1beta4: %v", err)
	}
	gotV1beta4 := joinV1beta4.(*kubeadmv1beta4.JoinConfiguration)
	if gotV1beta4.Discovery.BootstrapToken == nil {
		t.Fatal("expected v1beta4 bootstrap token discovery to be initialized")
	}
	if gotV1beta4.Timeouts == nil || gotV1beta4.Timeouts.Discovery == nil || gotV1beta4.Timeouts.Discovery.Duration != 5*time.Minute {
		t.Fatalf("unexpected v1beta4 discovery timeout: %#v", gotV1beta4.Timeouts)
	}
	if gotV1beta4.Discovery.TLSBootstrapToken != "abcdef.0123456789abcdef" {
		t.Fatalf("unexpected v1beta4 tls bootstrap token %q", gotV1beta4.Discovery.TLSBootstrapToken)
	}
}

func TestExportKubeadmConfigsPreservesCustomJoinDiscoveryTimeout(t *testing.T) {
	config := KubeadmConfig{}
	config.JoinConfiguration.Timeouts = &kubeadmv1beta4.Timeouts{
		Discovery: &metav1.Duration{Duration: 2 * time.Minute},
	}

	_, _, joinConfig, err := exportKubeadmConfigs(config, KubeadmV1beta3)
	if err != nil {
		t.Fatalf("export v1beta3: %v", err)
	}
	got := joinConfig.(*kubeadmv1beta3.JoinConfiguration)
	if got.Discovery.Timeout == nil || got.Discovery.Timeout.Duration != 2*time.Minute {
		t.Fatalf("unexpected discovery timeout: %#v", got.Discovery.Timeout)
	}
}

func TestLoadKubeadmConfigsNormalizesV1beta3ToV1beta4(t *testing.T) {
	const config = `
apiVersion: kubeadm.k8s.io/v1beta3
kind: ClusterConfiguration
kubernetesVersion: v1.30.9
apiServer:
  extraArgs:
    audit-log-format: json
`

	got, err := LoadKubeadmConfigs(config, utils.DecodeCRDFromString)
	if err != nil {
		t.Fatalf("LoadKubeadmConfigs: %v", err)
	}
	if len(got.ClusterConfiguration.APIServer.ExtraArgs) != 1 {
		t.Fatalf("got extraArgs %#v, want one arg", got.ClusterConfiguration.APIServer.ExtraArgs)
	}
	if got.ClusterConfiguration.APIServer.ExtraArgs[0].Name != "audit-log-format" ||
		got.ClusterConfiguration.APIServer.ExtraArgs[0].Value != "json" {
		t.Fatalf("unexpected extraArgs: %#v", got.ClusterConfiguration.APIServer.ExtraArgs)
	}
}

func TestLoadKubeadmConfigsRejectsUnsupportedKubeadmAPIVersion(t *testing.T) {
	const config = `
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
kubernetesVersion: v1.22.0
`

	_, err := LoadKubeadmConfigs(config, utils.DecodeCRDFromString)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported kubeadm ClusterConfiguration apiVersion kubeadm.k8s.io/v1beta2") {
		t.Fatalf("unexpected error: %v", err)
	}
}
