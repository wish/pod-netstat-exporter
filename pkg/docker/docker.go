package docker

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// ContainerToPID finds the PID of the given container
func ContainerToPID(hostMountPath, container string) (int, error) {
	raw := strings.Replace(container, "docker://", "", 1)
	return getPidForContainer(hostMountPath, raw)
}

// Everything below this point is modified from
// https://github.com/vishvananda/netns
// which according to the comments was mostly borrowed from
// the docker source code anyway
///////////////////////////////////////////////////////////////////////

// borrowed from docker/utils/utils.go
func findCgroupMountpoint(hostMountPath, cgroupType string) (string, error) {
	output, err := ioutil.ReadFile(hostMountPath + "/proc/mounts")
	if err != nil {
		return "", err
	}

	// /proc/mounts has 6 fields per line, one mount per line, e.g.
	// cgroup /sys/fs/cgroup/devices cgroup rw,relatime,devices 0 0
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, " ")
		if len(parts) == 6 && parts[2] == "cgroup" {
			for _, opt := range strings.Split(parts[3], ",") {
				if opt == cgroupType {
					return parts[1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("cgroup mountpoint not found for %s", cgroupType)
}

// Returns the relative path to the cgroup docker is running in.
// borrowed from docker/utils/utils.go
// modified to get the docker pid instead of using /proc/self
func getThisCgroup(hostMountPath, cgroupType string) (string, error) {
	dockerpid, err := ioutil.ReadFile(hostMountPath + "/var/run/docker.pid")
	if err != nil {
		return "", err
	}
	result := strings.Split(string(dockerpid), "\n")
	if len(result) == 0 || len(result[0]) == 0 {
		return "", fmt.Errorf("docker pid not found in %s/var/run/docker.pid", hostMountPath)
	}
	pid, err := strconv.Atoi(result[0])
	if err != nil {
		return "", err
	}
	output, err := ioutil.ReadFile(fmt.Sprintf(hostMountPath+"/proc/%d/cgroup", pid))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, ":")
		// any type used by docker should work
		if parts[1] == cgroupType {
			return parts[2], nil
		}
	}
	return "", fmt.Errorf("cgroup '%s' not found in %s/proc/%d/cgroup", cgroupType, hostMountPath, pid)
}

// Returns the first pid in a container.
// borrowed from docker/utils/utils.go
// modified to only return the first pid
// modified to glob with id
// modified to search for newer docker containers
func getPidForContainer(hostMountPath, id string) (int, error) {
	pid := 0

	// memory is chosen randomly, any cgroup used by docker works
	cgroupType := "memory"

	cgroupRoot, err := findCgroupMountpoint(hostMountPath, cgroupType)
	if err != nil {
		return pid, err
	}

	cgroupThis, err := getThisCgroup(hostMountPath, cgroupType)
	if err != nil {
		return pid, err
	}

	id += "*"

	attempts := []string{
		// Kubernetes with docker and CNI is even more different
		filepath.Join(hostMountPath, cgroupRoot, "..", "systemd", "kubepods", "*", "pod*", id, "tasks"),
		// Another flavor of containers location in recent kubernetes 1.11+
		filepath.Join(hostMountPath, cgroupRoot, cgroupThis, "kubepods.slice", "kubepods-besteffort.slice", "*", "docker-"+id+".scope", "tasks"),
		// When runs inside of a container with recent kubernetes 1.11+
		filepath.Join(hostMountPath, cgroupRoot, "kubepods.slice", "kubepods-besteffort.slice", "*", "docker-"+id+".scope", "tasks"),
		filepath.Join(hostMountPath, cgroupRoot, cgroupThis, id, "tasks"),
		// With more recent lxc versions use, cgroup will be in lxc/
		filepath.Join(hostMountPath, cgroupRoot, cgroupThis, "lxc", id, "tasks"),
		// With more recent docker, cgroup will be in docker/
		filepath.Join(hostMountPath, cgroupRoot, cgroupThis, "docker", id, "tasks"),
		// Even more recent docker versions under systemd use docker-<id>.scope/
		filepath.Join(hostMountPath, cgroupRoot, "system.slice", "docker-"+id+".scope", "tasks"),
		// Even more recent docker versions under cgroup/systemd/docker/<id>/
		filepath.Join(hostMountPath, cgroupRoot, "..", "systemd", "docker", id, "tasks"),
	}

	var filename string
	for _, attempt := range attempts {
		filenames, _ := filepath.Glob(attempt)
		if len(filenames) > 1 {
			return pid, fmt.Errorf("Ambiguous id supplied: %v", filenames)
		} else if len(filenames) == 1 {
			filename = filenames[0]
			break
		}
	}

	logrus.Tracef("Looking for container %v pid in %v", id, filename)

	if filename == "" {
		return pid, fmt.Errorf("Unable to find container: %v", id[:len(id)-1])
	}

	output, err := ioutil.ReadFile(filename)
	if err != nil {
		return pid, err
	}

	result := strings.Split(string(output), "\n")
	if len(result) == 0 || len(result[0]) == 0 {
		return pid, fmt.Errorf("No pid found for container")
	}

	pid, err = strconv.Atoi(result[0])
	if err != nil {
		return pid, fmt.Errorf("Invalid pid '%s': %s", result[0], err)
	}

	return pid, nil
}
