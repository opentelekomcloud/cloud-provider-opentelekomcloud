package elb

import (
	"context"
	"fmt"
	"strings"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/ipgroups"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/listeners"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// reconcileIPGroup enforces spec.loadBalancerSourceRanges on a listener via
// an ELB IP address group (whitelist).
func (lb *LoadBalancer) reconcileIPGroup(ctx context.Context, client *golangsdk.ServiceClient, lbID string, listener *listeners.Listener, name string, service *v1.Service, port v1.ServicePort) error {
	ranges := normalizeRanges(service.Spec.LoadBalancerSourceRanges)
	groupName := cutString(fmt.Sprintf("%s_%s_%d", name, port.Protocol, port.Port), maxResourceNameLength)

	attachedID := listener.IpGroup.IpGroupID

	if len(ranges) == 0 {
		// No restriction wanted: disable our whitelist if one is attached.
		// The SDK cannot fully detach a group, so it is kept (disabled)
		// and removed together with the listener.
		if attachedID == "" {
			return nil
		}
		group, err := ipgroups.Get(client, attachedID)
		if err != nil {
			if isNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get ipgroup %s: %w", attachedID, err)
		}
		if group.Name != groupName || listener.IpGroup.Enable == nil || !*listener.IpGroup.Enable {
			return nil // foreign group or already disabled
		}
		disabled := false
		err = mutate(ctx, client, lbID, func() error {
			_, err := listeners.Update(client, listener.ID, listeners.UpdateOpts{
				IpGroup: &listeners.IpGroupUpdate{IpGroupId: attachedID, Enable: &disabled},
			}).Extract()
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to disable ipgroup on listener %s: %w", listener.ID, err)
		}
		klog.V(2).Infof("Disabled source-range whitelist on listener %s", listener.ID)
		return nil
	}

	if attachedID != "" {
		group, err := ipgroups.Get(client, attachedID)
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to get ipgroup %s: %w", attachedID, err)
		}
		if err == nil {
			if group.Name != groupName {
				return fmt.Errorf("listener %s already has a foreign ipgroup %s; remove it or drop loadBalancerSourceRanges", listener.ID, group.ID)
			}
			if !ipListEqual(group.IpList, ranges) {
				err := retryOnConflict(ctx, lbID, func() error {
					return ipgroups.Update(client, group.ID, ipgroups.UpdateOpts{IpList: ipList(ranges)})
				})
				if err != nil {
					return fmt.Errorf("failed to update ipgroup %s: %w", group.ID, err)
				}
				klog.V(2).Infof("Updated source ranges of ipgroup %s", group.ID)
			}
			// Re-enable in case the whitelist was disabled earlier.
			if listener.IpGroup.Enable == nil || !*listener.IpGroup.Enable {
				return lb.attachIPGroup(ctx, client, lbID, listener.ID, group.ID)
			}
			return nil
		}
		// Attached group vanished: fall through and create a new one.
	}

	group, err := ipgroups.Create(client, ipgroups.CreateOpts{
		Name:        groupName,
		Description: fmt.Sprintf("Kubernetes service %s/%s", service.Namespace, service.Name),
		IpList:      ipList(ranges),
	})
	if err != nil {
		return fmt.Errorf("failed to create ipgroup for listener %s: %w", listener.ID, err)
	}
	klog.V(2).Infof("Created ipgroup %s (%s)", group.Name, group.ID)
	return lb.attachIPGroup(ctx, client, lbID, listener.ID, group.ID)
}

func (lb *LoadBalancer) attachIPGroup(ctx context.Context, client *golangsdk.ServiceClient, lbID, listenerID, groupID string) error {
	enabled := true
	err := mutate(ctx, client, lbID, func() error {
		_, err := listeners.Update(client, listenerID, listeners.UpdateOpts{
			IpGroup: &listeners.IpGroupUpdate{IpGroupId: groupID, Enable: &enabled, Type: "white"},
		}).Extract()
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to attach ipgroup %s to listener %s: %w", groupID, listenerID, err)
	}
	klog.V(2).Infof("Attached ipgroup %s to listener %s", groupID, listenerID)
	return nil
}

// deleteIPGroup removes the listener's IP group if this Service owns it.
// Call after the listener is deleted, when the group is no longer bound.
func (lb *LoadBalancer) deleteIPGroup(ctx context.Context, client *golangsdk.ServiceClient, lbID, groupID, name string) error {
	if groupID == "" {
		return nil
	}
	group, err := ipgroups.Get(client, groupID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get ipgroup %s: %w", groupID, err)
	}
	if !strings.HasPrefix(group.Name, name+"_") {
		return nil // not ours
	}
	err = retryOnConflict(ctx, lbID, func() error {
		if err := ipgroups.Delete(client, groupID); err != nil && !isNotFound(err) {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete ipgroup %s: %w", groupID, err)
	}
	klog.V(2).Infof("Deleted ipgroup %s", groupID)
	return nil
}

func normalizeRanges(ranges []string) []string {
	var out []string
	for _, r := range ranges {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

func ipList(ranges []string) *[]ipgroups.IpGroupOption {
	list := make([]ipgroups.IpGroupOption, 0, len(ranges))
	for _, r := range ranges {
		list = append(list, ipgroups.IpGroupOption{Ip: r})
	}
	return &list
}

func ipListEqual(current []ipgroups.IpInfo, ranges []string) bool {
	if len(current) != len(ranges) {
		return false
	}
	have := make(map[string]bool, len(current))
	for _, ip := range current {
		have[ip.Ip] = true
	}
	for _, r := range ranges {
		if !have[r] {
			return false
		}
	}
	return true
}
