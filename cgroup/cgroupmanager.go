package cgroup

import (
	"docker-my/cgroup/subsystem"
	"docker-my/container"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

type CgroupManager struct {
	// the path of the hierarchy in the cgroup,just like create the file-to the root-group
	Path string
	//resource config
	Resource *subsystem.ResourceConfig
}

func NewCGroupManager(path string) *CgroupManager {
	return &CgroupManager{
		Path: path,
	}
}

func (c *CgroupManager) Apply(pid int) error {
	for _, subSysIns := range subsystem.SubSystemIns {
		subSysIns.Apply(c.Path, pid)
	}
	return nil
}

func (c *CgroupManager) Set(res *subsystem.ResourceConfig) error {
	for _, subSysIns := range subsystem.SubSystemIns {
		subSysIns.Set(c.Path, res)
	}
	return nil
}

func (c *CgroupManager) Destroy() error {
	for _, subSysIns := range subsystem.SubSystemIns {
		if err := subSysIns.Remove(c.Path); err != nil {
			logrus.Warnf("remove cgroup fail %v", err)
		}
	}
	return nil
}

func Run(tty bool, comArray []string, res *subsystem.ResourceConfig) {
	parent, writePipe := container.NewParentProcess(tty)
	if parent == nil {
		logrus.Errorf("New parent process error")
		return
	}
	if err := parent.Start(); err != nil {
		logrus.Error(err)
	}
	//use docker-cgroup as cgroup name
	//create cgroupmanager,and use the apply and set for the resource limit
	cgroupManager := NewCGroupManager("mydocker-cgroup")
	defer cgroupManager.Destroy()
	//set the resource limit
	cgroupManager.Set(res)
	//add the docker process to the cgroup
	cgroupManager.Apply(parent.Process.Pid)
	//init the docker
	sendInitCommand(comArray, writePipe)

	parent.Wait()
}

func sendInitCommand(comArray []string, writePipe *os.File) {
	command := strings.Join(comArray, " ")
	logrus.Infof("command all is %s", command)
	writePipe.WriteString(command)
	writePipe.Close()
}
