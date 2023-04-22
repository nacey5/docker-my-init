package subsystem

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
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
	}else {
		return err
	}

}

//remove the cgroupPath to the cgroup
func (s *MemorySubSystem) Remove(cgroupPath string) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		return os.Remove(subsysCgroupPath)
	}else {
		return err
	}
}

//add a process to the cgroup for the cgroupPath
func (s *MemorySubSystem) Apply(cgroupPath string, pid int) error {
	if subsysCgroupPath, err := GetCgroupPath(s.Name(), cgroupPath, false); err == nil {
		//write the pid to the "task" file
		if err := os.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644);err!=nil{
			return fmt.Errorf("set cgroup proc fail %v",err)
		}
		return nil
	}else {
		return fmt.Errorf("get cgroup %s error: %v",cgroupPath,err)
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
	for scanner.Scan(){
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		for _, opt := range strings.Split(fields[len(fields)-1], ",") {
			if opt == subsystem {
				return fields[4]
			}
		}
	}
	if err := scanner.Err();err!=nil{
		return ""
	}
	return ""
}

//get the absolute path
func GetCgroupPath(subsystem string, cgroupPath string, autoCreate bool) (string, error) {
	cgroupRoot:=FindCgroupMountpoint(subsystem)
	if _, err := os.Stat(path.Join(cgroupRoot, cgroupPath));err==nil ||(autoCreate && os.IsNotExist(err)){
		if os.IsNotExist(err) {
			if err := os.Mkdir(path.Join(cgroupRoot, cgroupPath), 0755);err==nil{

			}else {
				return "",fmt.Errorf("error create cgroup %v",err)
			}
			return path.Join(cgroupRoot,cgroupPath),nil
		}else {
			return "",fmt.Errorf("cgroup path error %v",err)
		}
	}
}
