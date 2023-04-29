package subsystem

import (
	"bufio"
	"docker-my/container"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type ResourceConfig struct {
	MemoryLimit string
	CpuShare    string
	CpuSet      string
}

type SubSystem interface {
	//return the name of subsystem
	Name() string
	//set the cgroup on this subsystem
	Set(path string, res *ResourceConfig) error
	//add the process to the cgroup
	Apply(path string, pid int) error
	//remove some cgroup
	Remove(path string) error
}

var (
	SubSystemIns = []SubSystem{
		&CpusetSubSystem{},
		&MemorySubSystem{},
		&CpuSubSystem{},
	}
)

type MemorySubSystem struct {
}

// Set the memory resouce limit
func (s *MemorySubSystem) Set(cgroupPath string, res *ResourceConfig) error {
	// get the path of the subsystem resource
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, true); err == nil {
		//set the cgroup resource limit
		if err := os.WriteFile(path.Join(subsysCgroupPath, "memory.limit_in_bytes"), []byte(res.MemoryLimit), 0644); err != nil {
			return fmt.Errorf("set cgroup memory fail %v", err)
		}
		return nil
	} else {
		return err
	}

}

// remove the cgroupPath to the cgroup
func (s *MemorySubSystem) Remove(cgroupPath string) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		return os.Remove(subsysCgroupPath)
	} else {
		return err
	}
}

// add a process to the cgroup for the cgroupPath
func (s *MemorySubSystem) Apply(cgroupPath string, pid int) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		//write the pid to the "task" file
		if err := os.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			return fmt.Errorf("set cgroup proc fail %v", err)
		}
		return nil
	} else {
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}

// return the cgroup name
func (s *MemorySubSystem) Name() string {
	return "memory"
}

// get the cgroup by the "/proc/self/mountinfo"
func FindCgroupMountpoint(subsystem string) string {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		for _, opt := range strings.Split(fields[len(fields)-1], ",") {
			if opt == subsystem {
				return fields[4]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ""
	}
	return ""
}

// get the absolute path
func GetCgroupPath(subsystem string, cgroupPath string, autoCreate bool) (string, error) {
	cgroupRoot := FindCgroupMountpoint(subsystem)
	if _, err := os.Stat(path.Join(cgroupRoot, cgroupPath)); err == nil || (autoCreate && os.IsNotExist(err)) {
		if os.IsNotExist(err) {
			if err := os.Mkdir(path.Join(cgroupRoot, cgroupPath), 0755); err == nil {
			} else {
				return "", fmt.Errorf("error create cgroup %v", err)
			}
			return path.Join(cgroupRoot, cgroupPath), nil
		} else {
			return "", fmt.Errorf("cgroup path error %v", err)
		}
	}
	return "", nil
}

func pivotRoot(root string) error {
	// for the new root,remount the new root
	if err := syscall.Mount(root, root, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("Mount rootfs to itself error: %v", err)
	}
	// create rootfs/.pivot_root storage the old_root
	pivotDir := filepath.Join(root, ".pivot_root")
	if err := os.Mkdir(pivotDir, 0777); err != nil {
		return err
	}
	//pivot_root to the new rootfs
	if err := syscall.PivotRoot(root, pivotDir); err != nil {
		return fmt.Errorf("pivot_root %v", err)
	}
	//edit the new work dir to the root
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir / %v", err)
	}

	pivotDir = filepath.Join("/", ".pivot_root")
	//umount rootfs/ .pivot_root
	if err := syscall.Unmount(pivotDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("umount pivot_root dir %v", err)
	}
	//remove the temp file
	return os.Remove(pivotDir)
}

func setUpMount() {
	pwd, err := os.Getwd()
	if err != nil {
		log.Errorf("Get current location error")
		return
	}
	log.Infof("Current location is %s", pwd)
	pivotRoot(pwd)

	//mount proc
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV
	syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlags), "")
	syscall.Mount("tmpfs", "/dev", "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=75")
}

func NewWorkSpace(volume, imageName, containerName string) {
	CreateReadOnlyLayer(container.RootUrl)
	CreateWriteLayer(container.RootUrl)
	CreateMountPoint(container.RootUrl, container.MntUrl)
	CreateReadOnlyLayer(imageName)
	CreateWriteLayer(containerName)
	CreateMountPoint(containerName, imageName)
	if volume != "" {
		//analytic the volume
		volumeURLS := volumeUrlExtract(volume)
		length := len(volumeURLS)
		if length == 2 && volumeURLS[0] != "" && volumeURLS[1] != "" {
			//mount the data volume
			MountVolume(container.RootUrl, container.MntUrl, volumeURLS)
			log.Infof("%q", volumeURLS)
		} else {
			log.Infof("Volume parameter input is not correct.")
		}
	}
	MountVolume(container.RootUrl, container.MntUrl, volume)
}

func MountVolume(rootURL string, mntURL string, volumeURLs []string) {
	//create host flooder
	parentUrl := volumeURLs[0]
	if err := os.Mkdir(parentUrl, 0777); err != nil {
		log.Infof("Mkdir parent dir %s error. %v", parentUrl, err)
	}
	//mount the point into the volume flooder
	containeerUrl := volumeURLs[1]
	containeerVolumeURL := mntURL + containeerUrl
	if err := os.Mkdir(containeerVolumeURL, 0777); err != nil {
		log.Infof("Mkdir container dir %s error. %v", containeerUrl, err)
	}
	//put the host mount the volume
	dirs := "dirs=" + parentUrl
	cmd := exec.Command("mount", "-t", "aufs", "-o", dirs, "none", containeerVolumeURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("Mount volume failed %v.", err)
	}
}

// analytic the volume's string
func volumeUrlExtract(volume string) []string {
	var volumeURLs []string
	volumeURLs = strings.Split(volume, ":")
	return volumeURLs
}

// to busybox.tar unzip to the file busybox,and the container as the readonly layer
func CreateReadOnlyLayer(rootURL string) {
	busyboxURL := rootURL + "busybox/"
	busyboxTarURL := rootURL + "busybox.tar"
	exist, err := PathExists(busyboxURL)
	if err != nil {
		log.Infof("Fail to judge whether dir %s exist. %v", busyboxURL, err)
	}
	if !exist {
		if err := os.Mkdir(busyboxTarURL, 0777); err != nil {
			log.Infof("Mkdir dir %s error. %v", busyboxURL, err)
		}
		if _, err := exec.Command("tar", "-xvf", busyboxTarURL, "-C", busyboxURL).CombinedOutput(); err != nil {
			log.Errorf("unTar dir %s error %v", busyboxTarURL, err)
		}
	}
}

// create the writelayer as the container's only layer
func CreateWriteLayer(rootURL string) {
	writeURL := rootURL + "writeLayer/"
	if err := os.Mkdir(writeURL, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", writeURL, err)
	}
}

func CreateMountPoint(rootURL string, mntURL string) {
	//create the file mnt as the mount point
	if err := os.Mkdir(mntURL, 0777); err != nil {
		log.Error("Mkdir dir %s error. %v", mntURL, err)
	}
	//put the writeLayer and busybo file and the mount to the mnt
	dirs := "dirs=" + rootURL + "writeLayer:" + rootURL + "busybox"
	cmd := exec.Command("mount", "-t", "aufs", "-o", dirs, "none", mntURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("%v", err)
	}
}

// judge the path is exists or not
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func DeleteMountPoint(rootURL string, mntURL string) {
	cmd := exec.Command("umount", mntURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("%v", err)
	}
	if err := os.RemoveAll(mntURL); err != nil {
		log.Errorf("Remove dir %s error %v", mntURL, err)
	}
}

func DeleteWriteLayer(rootURL string) {
	writeURL := rootURL + "writeLayer/"
	if err := os.RemoveAll(writeURL); err != nil {
		log.Errorf("Remove dir %s error %v", writeURL, err)
	}
}
