package sshhelper

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

//SSHContext 类
type SSHContext struct {
	clientConfig *ssh.ClientConfig
	addr         string
	client       *ssh.Client
	sftpClient   *sftp.Client
}

//New 新建ssh服务
func New(user, password, host string, port int) *SSHContext {
	// get auth method
	auth := make([]ssh.AuthMethod, 0)
	auth = append(auth, ssh.Password(password))

	clientConfig := &ssh.ClientConfig{
		User:    user,
		Auth:    auth,
		Timeout: 30 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	return &SSHContext{
		clientConfig: clientConfig,
		addr:         addr,
	}
}

//Connect 连接
func (context *SSHContext) Connect() error {
	if context.client != nil {
		return nil
	}

	var err error
	if context.client, err = ssh.Dial("tcp", context.addr, context.clientConfig); err != nil {
		return err
	}

	return nil
}

//Close 关闭
func (context *SSHContext) Close() {
	if context.sftpClient != nil {
		context.sftpClient.Close()
		context.sftpClient = nil
	}

	if context.client != nil {
		context.client.Close()
		context.client = nil
	}
}

//SSHWriter 发送
type SSHWriter struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (w *SSHWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

//Get ...
func (w *SSHWriter) Get() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.b.Bytes())
}

//Exec 执行
func (context *SSHContext) Exec(command string) (string, error) {
	var session *ssh.Session
	var err error
	if err = context.Connect(); err != nil {
		return "", err
	}

	if session, err = context.client.NewSession(); err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)

	if err != nil {
		return "", err
	}

	return string(output), nil
}

//ExecWithWriter ...
func (context *SSHContext) ExecWithWriter(command string, writer *SSHWriter) error {
	var session *ssh.Session
	var err error
	if err = context.Connect(); err != nil {
		return err
	}

	if session, err = context.client.NewSession(); err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = writer
	session.Stderr = writer
	err = session.Run(command)

	if err != nil {
		return err
	}

	return nil
}

//Copy ...
func (context *SSHContext) Copy(localFilePath, remoteDir string, percent *int64, stop *bool) error {
	var err error
	if err = context.Connect(); err != nil {
		return err
	}

	if context.sftpClient == nil {
		if context.sftpClient, err = sftp.NewClient(context.client); err != nil {
			return err
		}
	}

	srcFile, err := os.Open(localFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	var remoteFileName = path.Base(localFilePath)
	dstFile, err := context.sftpClient.Create(path.Join(remoteDir, remoteFileName))
	if err != nil {
		return err
	}
	defer dstFile.Close()

	stat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	var sended int64 = 0
	total := stat.Size()

	buf := make([]byte, 256*1024)
	for (stop == nil) || !(*stop) {
		n, _ := srcFile.Read(buf)
		if n == 0 {
			break
		}
		var _, err = dstFile.Write(buf[0:n])
		if err != nil {
			return err
		}

		sended += int64(n)
		if percent != nil {
			if total > 0 {
				*percent = sended * 10000 / total
			}
		}
	}

	if percent != nil {
		*percent = 10000
	}
	return nil
}
