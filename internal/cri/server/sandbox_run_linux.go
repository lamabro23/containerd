/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package server

import (
	"fmt"
	"syscall"

	"github.com/containerd/containerd/v2/pkg/netns"
	"github.com/containerd/containerd/v2/pkg/sys"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *criService) bringUpLoopback(netns string) error {
	if err := ns.WithNetNSPath(netns, func(_ ns.NetNS) error {
		link, err := netlink.LinkByName("lo")
		if err != nil {
			return err
		}
		return netlink.LinkSetUp(link)
	}); err != nil {
		return fmt.Errorf("error setting loopback interface up: %w", err)
	}
	return nil
}

func (c *criService) setupNetnsWithinUserns(netnsMountDir string, opt *runtime.UserNamespace) (*netns.NetNS, error) {
	if opt.GetMode() != runtime.NamespaceMode_POD {
		return nil, fmt.Errorf("required pod-level user namespace setting")
	}

	uidMaps := opt.GetUids()
	if len(uidMaps) == 0 {
		return nil, fmt.Errorf("required at least one uid mapping, but got empty uid mapping")
	}

	gidMaps := opt.GetGids()
	if len(gidMaps) == 0 {
		return nil, fmt.Errorf("required at least one gid mapping, but got empty gid mapping")
	}

	if len(uidMaps) != len(gidMaps) {
		return nil, fmt.Errorf("uid mapping and gid mapping should have the same length")
	}

	var netNs *netns.NetNS
	var err error
	uidMapsString := ""
	gidMapsString := ""
	for i := range uidMaps {
		uidMapsString += fmt.Sprintf("%d:%d:%d,", uidMaps[i].ContainerId, uidMaps[i].HostId, uidMaps[i].Length)
		gidMapsString += fmt.Sprintf("%d:%d:%d,", gidMaps[i].ContainerId, gidMaps[i].HostId, gidMaps[i].Length)
	}

	uerr := sys.UnshareAfterEnterUserns(
		uidMapsString[:len(uidMapsString)-1],
		gidMapsString[:len(gidMapsString)-1],
		syscall.CLONE_NEWNET,
		func(pid int) error {
			netNs, err = netns.NewNetNSFromPID(netnsMountDir, uint32(pid))
			if err != nil {
				return fmt.Errorf("failed to mount netns from pid %d: %w", pid, err)
			}
			return nil
		},
	)
	if uerr != nil {
		return nil, uerr
	}
	return netNs, nil
}
