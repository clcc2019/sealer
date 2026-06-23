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
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"github.com/sealerio/sealer/common"
	utilsnet "github.com/sealerio/sealer/utils/net"
	osi "github.com/sealerio/sealer/utils/os"
	progressbar "github.com/sealerio/sealer/utils/progressbar"
	"github.com/sirupsen/logrus"
)

const (
	Md5sumCmd = "md5sum %s | cut -d\" \" -f1"
	rsyncCmd  = "rsync"
)

var (
	epuMap = &epuRWMap{epu: map[string]*progressbar.EasyProgressUtil{}}
)

type epuRWMap struct {
	sync.RWMutex
	epu map[string]*progressbar.EasyProgressUtil
}

func (m *epuRWMap) Get(k string) (*progressbar.EasyProgressUtil, bool) {
	m.RLock()
	defer m.RUnlock()
	v, existed := m.epu[k]
	return v, existed
}

func (m *epuRWMap) Set(k string, v *progressbar.EasyProgressUtil) {
	m.Lock()
	defer m.Unlock()
	m.epu[k] = v
}

// CopyR scp remote file to local
func (s *SSH) CopyR(host net.IP, localFilePath, remoteFilePath string) error {
	if utilsnet.IsLocalIP(host, s.LocalAddress) {
		if remoteFilePath != localFilePath {
			logrus.Debugf("copy local files: src %s to dst %s", remoteFilePath, localFilePath)
			return osi.RecursionCopy(remoteFilePath, localFilePath)
		}
		return nil
	}

	sftpClient, err := s.sftpConnect(host)
	if err != nil {
		return fmt.Errorf("failed to new sftp client: %v", err)
	}
	// open remote source file
	srcFile, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file(%s): %v", remoteFilePath, err)
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}()

	err = s.Fs.MkdirAll(filepath.Dir(localFilePath))
	if err != nil {
		return err
	}
	// open local Destination file
	dstFile, err := os.Create(filepath.Clean(localFilePath))
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}()
	// copy to local file
	_, err = srcFile.WriteTo(dstFile)
	return err
}

// Copy file or dir to remotePath, add md5 validate
func (s *SSH) Copy(host net.IP, localPath, remotePath string) error {
	if utilsnet.IsLocalIP(host, s.LocalAddress) {
		if localPath == remotePath {
			return nil
		}
		logrus.Debugf("copy local files: src %s to dst [%s]:%s", localPath, host, remotePath)
		return osi.RecursionCopy(localPath, remotePath)
	}
	logrus.Debugf("remote copy files src %s to dst [%s]:%s", localPath, host, remotePath)

	if err := s.copyWithRsync(host, localPath, remotePath); err == nil {
		return nil
	} else {
		logrus.Debugf("failed to copy files src %s to dst [%s]:%s with rsync, fallback to sftp: %v", localPath, host, remotePath, err)
	}

	sftpClient, err := s.sftpConnect(host)
	if err != nil {
		return fmt.Errorf("failed to new sftp client of host(%s): %s", host, err)
	}

	f, err := s.Fs.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get file stat of path(%s): %s", localPath, err)
	}

	baseRemoteFilePath := filepath.Dir(remotePath)
	_, err = sftpClient.ReadDir(baseRemoteFilePath)
	if err != nil {
		if err = sftpClient.MkdirAll(baseRemoteFilePath); err != nil {
			return err
		}
	}
	number := 1
	if f.IsDir() {
		number = osi.CountDirFiles(localPath)
	}
	// no file in dir, do need to send
	if number == 0 {
		return nil
	}

	epu, ok := epuMap.Get(host.String())
	if !ok {
		epu = progressbar.NewEasyProgressUtil(number, fmt.Sprintf("[copying files to %s]", host))
		epuMap.Set(host.String(), epu)
	} else {
		epu.SetTotal(epu.GetMax() + number)
	}

	if f.IsDir() {
		s.copyLocalDirToRemote(host, sftpClient, localPath, remotePath, epu)
	} else {
		err = s.copyLocalFileToRemote(host, sftpClient, localPath, remotePath)
		if err != nil {
			epu.Fail(err)
		}
		epu.Increment()
	}
	return nil
}

func (s *SSH) copyWithRsync(host net.IP, localPath, remotePath string) error {
	if err := s.checkRsyncPrerequisites(host); err != nil {
		return err
	}

	f, err := s.Fs.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get file stat of path(%s): %s", localPath, err)
	}

	srcPath := localPath
	dstPath := remotePath
	mkdirPath := filepath.Dir(remotePath)
	if f.IsDir() {
		srcPath = pathWithTrailingSeparator(filepath.Clean(localPath))
		dstPath = pathWithTrailingSeparator(remotePath)
		mkdirPath = remotePath
	}

	if err := s.CmdAsync(host, nil, fmt.Sprintf("mkdir -p %s", shellQuote(mkdirPath))); err != nil {
		return fmt.Errorf("failed to create remote path %s on host %s: %v", mkdirPath, host, err)
	}

	args := []string{
		"-a",
		"--checksum",
		"--no-owner",
		"--no-group",
		"-e",
		s.rsyncSSHCommand(),
		srcPath,
		s.rsyncRemoteTarget(host, dstPath),
	}
	if s.User != common.ROOT {
		args = append([]string{"--rsync-path=sudo rsync"}, args...)
	}

	logrus.Debugf("remote rsync files src %s to dst [%s]:%s", localPath, host, remotePath)

	cmd := exec.Command(rsyncCmd, args...) // #nosec G204
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String() + stderr.String())
		if output != "" {
			return fmt.Errorf("rsync failed: %v: %s", err, output)
		}
		return fmt.Errorf("rsync failed: %v", err)
	}

	return nil
}

func (s *SSH) checkRsyncPrerequisites(host net.IP) error {
	if _, err := exec.LookPath(rsyncCmd); err != nil {
		return fmt.Errorf("local rsync command not found: %v", err)
	}

	if s.Password != "" && s.PkFile == "" {
		return fmt.Errorf("rsync over ssh is skipped for password-only authentication")
	}

	if err := s.CmdAsync(host, nil, "command -v rsync >/dev/null 2>&1"); err != nil {
		return fmt.Errorf("remote rsync command not available on host %s: %v", host, err)
	}

	return nil
}

func (s *SSH) rsyncSSHCommand() string {
	port := s.Port
	if port == "" {
		port = DefaultSSHPort
	}

	args := []string{
		"ssh",
		"-p", port,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "BatchMode=yes",
	}
	if s.PkFile != "" {
		args = append(args, "-i", s.PkFile)
	}

	for i, arg := range args {
		args[i] = shellQuote(arg)
	}
	return strings.Join(args, " ")
}

func (s *SSH) rsyncRemoteTarget(host net.IP, remotePath string) string {
	userPrefix := ""
	if s.User != "" {
		userPrefix = s.User + "@"
	}
	return fmt.Sprintf("%s%s:%s", userPrefix, rsyncHost(host), shellQuote(remotePath))
}

func rsyncHost(host net.IP) string {
	hostString := host.String()
	if strings.Contains(hostString, ":") {
		return "[" + hostString + "]"
	}
	return hostString
}

func pathWithTrailingSeparator(p string) string {
	if strings.HasSuffix(p, string(os.PathSeparator)) {
		return p
	}
	return p + string(os.PathSeparator)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (s *SSH) remoteMd5Sum(host net.IP, remoteFilePath string) string {
	cmd := fmt.Sprintf(Md5sumCmd, remoteFilePath)
	remoteMD5, err := s.CmdToString(host, nil, cmd, "")
	if err != nil {
		logrus.Errorf("failed to count md5 of remote file(%s) on host(%s): %v", remoteFilePath, host, err)
	}
	return strings.ReplaceAll(remoteMD5, "\r", "")
}

func (s *SSH) copyLocalDirToRemote(host net.IP, sftpClient *sftp.Client, localPath, remotePath string, epu *progressbar.EasyProgressUtil) {
	localFiles, err := os.ReadDir(localPath)
	if err != nil {
		logrus.Errorf("failed to read local path dir(%s) on host(%s): %s", localPath, host, err)
		return
	}
	if err = sftpClient.MkdirAll(remotePath); err != nil {
		logrus.Errorf("failed to create remote path %s: %v", remotePath, err)
		return
	}
	for _, file := range localFiles {
		lfp := path.Join(localPath, file.Name())
		rfp := path.Join(remotePath, file.Name())
		if file.IsDir() {
			if err = sftpClient.MkdirAll(rfp); err != nil {
				logrus.Errorf("failed to create remote path %s: %v", rfp, err)
				return
			}
			s.copyLocalDirToRemote(host, sftpClient, lfp, rfp, epu)
		} else {
			err := s.copyLocalFileToRemote(host, sftpClient, lfp, rfp)
			if err != nil {
				errMsg := fmt.Sprintf("failed to copy local file(%s) to remote(%s) on host(%s): %v", lfp, rfp, host, err)
				epu.Fail(err)
				logrus.Error(errMsg)
				return
			}
			epu.Increment()
		}
	}
}

// check the remote file existence before copying
func (s *SSH) copyLocalFileToRemote(host net.IP, sftpClient *sftp.Client, localPath, remotePath string) error {
	var (
		srcMd5, dstMd5 string
	)
	srcMd5 = localMd5Sum(localPath)
	if exist, err := s.IsFileExist(host, remotePath); err != nil {
		return err
	} else if exist {
		dstMd5 = s.remoteMd5Sum(host, remotePath)
		if srcMd5 == dstMd5 {
			logrus.Debugf("remote dst %s already exists and is the latest version , skip copying process", remotePath)
			return nil
		}
	}

	srcFile, err := os.Open(filepath.Clean(localPath))
	if err != nil {
		return err
	}
	defer func() {
		if err := srcFile.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}()

	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	fileStat, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stat: %v", err)
	}
	// TODO seems not work
	if err := dstFile.Chmod(fileStat.Mode()); err != nil {
		return fmt.Errorf("failed to chmod remote file: %v", err)
	}
	defer func() {
		if err := dstFile.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}()
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	dstMd5 = s.remoteMd5Sum(host, remotePath)
	if srcMd5 != dstMd5 {
		return fmt.Errorf("[ssh][%s] failed to validate md5sum: (%s != %s)", host, srcMd5, dstMd5)
	}
	return nil
}

// RemoteDirExist if remote file not exist return false and nil
func (s *SSH) RemoteDirExist(host net.IP, remoteDirPath string) (bool, error) {
	sftpClient, err := s.sftpConnect(host)
	if err != nil {
		return false, fmt.Errorf("new sftp client failed %v", err)
	}

	if _, err := sftpClient.ReadDir(remoteDirPath); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SSH) IsFileExist(host net.IP, remoteFilePath string) (bool, error) {
	sftpClient, err := s.sftpConnect(host)
	if err != nil {
		return false, fmt.Errorf("failed to new sftp client of host(%s): %s", host, err)
	}

	_, err = sftpClient.Stat(remoteFilePath)
	if err == os.ErrNotExist {
		return false, nil
	}
	return err == nil, err
}
