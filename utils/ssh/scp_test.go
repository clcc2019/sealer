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

package ssh

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	cryptossh "golang.org/x/crypto/ssh"
)

func TestRsyncRemoteTarget(t *testing.T) {
	tests := []struct {
		name       string
		ssh        SSH
		host       net.IP
		remotePath string
		want       string
	}{
		{
			name:       "ipv4 with user",
			ssh:        SSH{User: "root"},
			host:       net.ParseIP("192.168.0.2"),
			remotePath: "/var/lib/sealer/rootfs/",
			want:       "root@192.168.0.2:'/var/lib/sealer/rootfs/'",
		},
		{
			name:       "ipv6 with user",
			ssh:        SSH{User: "root"},
			host:       net.ParseIP("2001:db8::1"),
			remotePath: "/var/lib/sealer/rootfs/",
			want:       "root@[2001:db8::1]:'/var/lib/sealer/rootfs/'",
		},
		{
			name:       "without user",
			ssh:        SSH{},
			host:       net.ParseIP("192.168.0.2"),
			remotePath: "/tmp/file",
			want:       "192.168.0.2:'/tmp/file'",
		},
		{
			name:       "quoted remote path",
			ssh:        SSH{User: "root"},
			host:       net.ParseIP("192.168.0.2"),
			remotePath: "/tmp/it's here/file",
			want:       "root@192.168.0.2:'/tmp/it'\\''s here/file'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ssh.rsyncRemoteTarget(tt.host, tt.remotePath))
		})
	}
}

func TestRsyncSSHCommand(t *testing.T) {
	t.Run("default port", func(t *testing.T) {
		cmd := (&SSH{}).rsyncSSHCommand()
		assert.Contains(t, cmd, "'ssh'")
		assert.Contains(t, cmd, "'-p' '22'")
		assert.Contains(t, cmd, "'BatchMode=yes'")
	})

	t.Run("custom port and private key", func(t *testing.T) {
		cmd := (&SSH{Port: "2222", PkFile: "/tmp/key with space"}).rsyncSSHCommand()
		assert.Contains(t, cmd, "'-p' '2222'")
		assert.Contains(t, cmd, "'-i' '/tmp/key with space'")
	})
}

func TestPathWithTrailingSeparator(t *testing.T) {
	assert.Equal(t, "/tmp/source/", pathWithTrailingSeparator("/tmp/source"))
	assert.Equal(t, "/tmp/source/", pathWithTrailingSeparator("/tmp/source/"))
}

func TestEnsureLocalPublicKeyGeneratesKeyPair(t *testing.T) {
	privateKeyPath := filepath.Join(t.TempDir(), "id_rsa")
	sshClient := &SSH{PkFile: privateKeyPath}

	publicKey, err := sshClient.ensureLocalPublicKey()
	assert.NoError(t, err)
	assert.NotEmpty(t, publicKey)

	privateKeyData, err := os.ReadFile(privateKeyPath)
	if assert.NoError(t, err) {
		_, err = cryptossh.ParsePrivateKey(privateKeyData)
		assert.NoError(t, err)
	}

	_, _, _, _, err = cryptossh.ParseAuthorizedKey(publicKey)
	assert.NoError(t, err)

	privateKeyStat, err := os.Stat(privateKeyPath)
	if assert.NoError(t, err) {
		assert.Equal(t, os.FileMode(0600), privateKeyStat.Mode().Perm())
	}

	publicKeyStat, err := os.Stat(privateKeyPath + ".pub")
	if assert.NoError(t, err) {
		assert.Equal(t, os.FileMode(0644), publicKeyStat.Mode().Perm())
	}
}

func TestAuthorizedKeyInstallCommand(t *testing.T) {
	publicKey := "ssh-rsa AAAATEST user'with-quote"

	cmd := authorizedKeyInstallCommand(publicKey)

	assert.Contains(t, cmd, "mkdir -p \"$HOME/.ssh\"")
	assert.Contains(t, cmd, "{ grep -qxF 'ssh-rsa AAAATEST user'\\''with-quote' \"$HOME/.ssh/authorized_keys\" || printf '%s\\n' 'ssh-rsa AAAATEST user'\\''with-quote' >> \"$HOME/.ssh/authorized_keys\"; }")
	assert.Contains(t, cmd, "chmod 600 \"$HOME/.ssh/authorized_keys\"")
}

/*
func TestSSHCopyLocalToRemote(t *testing.T) {
	type args struct {
		host       string
		localPath  string
		remotePath string
	}
	var (
		host = "10.96.33.168"
		ssh  = SSH{
			User:       "root",
			Password:   "123456",
			PkFile:     "",
			PkPassword: "",
			Timeout:    nil,
		}
	)
	tests := []struct {
		name    string
		fields  SSH
		args    args
		wantErr bool
	}{
		{
			name:   "test for copy file to remote server",
			fields: ssh,
			args: args{
				host,
				"../test/file/01",
				"/home/temp/01",
			},
			wantErr: false,
		},
		{
			name:   "test copy for file when local file is not exist",
			fields: ssh,
			args: args{
				host,
				// local file  001 is not exist.
				"../test/file/123",
				"/home/temp/01",
			},
			wantErr: false,
		},
		{
			name:   "test copy dir to remote server",
			fields: ssh,
			args: args{
				host,
				"../test/file",
				"/home/temp011",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := &SSH{
				User:       tt.fields.User,
				Password:   tt.fields.Password,
				PkFile:     tt.fields.PkFile,
				PkPassword: tt.fields.PkPassword,
				Timeout:    tt.fields.Timeout,
				Fs:         fs.NewFilesystem(),
			}

			if !fileExist(tt.args.localPath) {
				logrus.Error("local filepath is not exit")
				return
			}
			//if ss.IsFileExist(host, tt.args.remotePath) {
			//	log.Error("remote filepath is exit")
			//	return
			//}
			// test copy dir
			err := ss.Copy(tt.args.host, tt.args.localPath, tt.args.remotePath)
			if (err != nil) != tt.wantErr {
				logrus.Error(err)
				t.Errorf("err: %v", err)
			}

			// test the copy result
			//ss.Cmd(tt.args.host, "ls -lh "+tt.args.remotePath)

			// rm remote file
			//ss.Cmd(tt.args.host, "rm -rf "+tt.args.remotePath)
		})
	}
}*/

//func TestSSHFetchRemoteToLocal(t *testing.T) {
//	type args struct {
//		host       net.IP
//		localPath  string
//		remotePath string
//	}
//	var (
//		host = net.IP{}
//		ssh  = SSH{
//			User:       "root",
//			Password:   "",
//			PkFile:     "",
//			PkPassword: "",
//			Timeout:    nil,
//		}
//	)
//	tests := []struct {
//		name    string
//		fields  SSH
//		args    args
//		wantErr bool
//	}{
//		{
//			name:   "test for fetch remote file to local",
//			fields: ssh,
//			args: args{
//				host,
//				"/root/.kube/config",
//				"/root/Clusterfile",
//			},
//			wantErr: false,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			ss := &SSH{
//				User:       tt.fields.User,
//				Password:   tt.fields.Password,
//				PkFile:     tt.fields.PkFile,
//				PkPassword: tt.fields.PkPassword,
//				Timeout:    tt.fields.Timeout,
//				Fs:         fs.NewFilesystem(),
//			}
//
//			if exist, err := ss.IsFileExist(host, tt.args.remotePath); err != nil {
//				logrus.Error("err: ", err)
//				return
//			} else if !exist {
//				logrus.Error("remote filepath is not exit")
//				return
//			}
//			err := ss.CopyR(tt.args.host, tt.args.localPath, tt.args.remotePath)
//			if (err != nil) != tt.wantErr {
//				logrus.Error(err)
//				t.Errorf("err: %v", err)
//			}
//		})
//	}
//}

/*
func TestSSH_Copy(t *testing.T) {
	type fields struct {
		User       string
		Password   string
		PkFile     string
		PkPassword string
	}
	type args struct {
		host       string
		localPath  string
		remotePath string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			"test copy dir",
			fields{
				User:       "root",
				Password:   "",
				PkFile:     "",
				PkPassword: "",
			},
			args{
				host:       "",
				localPath:  "./pkg/cert/pki",
				remotePath: "/root/kubernetes/pki",
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SSH{
				User:       tt.fields.User,
				Password:   tt.fields.Password,
				PkFile:     tt.fields.PkFile,
				PkPassword: tt.fields.PkPassword,
				Fs:         fs.NewFilesystem(),
			}
			if err := s.Copy(tt.args.host, tt.args.localPath, tt.args.remotePath); (err != nil) != tt.wantErr {
				t.Errorf("Copy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
*/
