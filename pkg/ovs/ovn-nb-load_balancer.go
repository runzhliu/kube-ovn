package ovs

import (
	"context"
	"fmt"

	"github.com/ovn-org/libovsdb/model"
	"github.com/ovn-org/libovsdb/ovsdb"

	ovsclient "github.com/kubeovn/kube-ovn/pkg/ovsdb/client"
	"github.com/kubeovn/kube-ovn/pkg/ovsdb/ovnnb"
)

// CreateLoadBalancer create loadbalancer
func (c *ovnClient) CreateLoadBalancer(lbName, protocol, selectFields string) error {
	exist, err := c.LoadBalancerExists(lbName)
	if err != nil {
		return err
	}

	// found, ignore
	if exist {
		return nil
	}

	lb := &ovnnb.LoadBalancer{
		UUID:     ovsclient.NamedUUID(),
		Name:     lbName,
		Protocol: &protocol,
	}

	if len(selectFields) != 0 {
		lb.SelectionFields = []string{selectFields}
	}

	op, err := c.ovnNbClient.Create(lb)
	if err != nil {
		return fmt.Errorf("generate operations for creating load balancer %s: %v", lbName, err)
	}

	if err := c.Transact("lb-add", op); err != nil {
		return fmt.Errorf("create load balancer %s: %v", lbName, err)
	}

	return nil
}

// DeleteLoadBalancers delete several loadbalancer once
func (c *ovnClient) DeleteLoadBalancers(lbs ...string) error {
	if len(lbs) == 0 {
		return nil
	}

	ops := make([]ovsdb.Operation, 0, len(lbs))

	for _, lbName := range lbs {
		op, err := c.DeleteLoadBalancerOp(lbName)
		if err != nil {
			return nil
		}

		// ingnore non-existent object
		if len(op) == 0 {
			continue
		}

		ops = append(ops, op...)
	}

	if err := c.Transact("lb-del", ops); err != nil {
		return fmt.Errorf("delete load balancers %v: %v", lbs, err)
	}

	return nil
}

// GetLoadBalancer get load balancer by name,
// it is because of lack name index that does't use ovnNbClient.Get
func (c *ovnClient) GetLoadBalancer(lbName string, ignoreNotFound bool) (*ovnnb.LoadBalancer, error) {
	lbList := make([]ovnnb.LoadBalancer, 0)
	if err := c.ovnNbClient.WhereCache(func(lb *ovnnb.LoadBalancer) bool {
		return lb.Name == lbName
	}).List(context.TODO(), &lbList); err != nil {
		return nil, fmt.Errorf("list load balancer %q: %v", lbName, err)
	}

	// not found
	if len(lbList) == 0 {
		if ignoreNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("not found load balancer %q", lbName)
	}

	if len(lbList) > 1 {
		return nil, fmt.Errorf("more than one load balancer with same name %q", lbName)
	}

	return &lbList[0], nil
}

func (c *ovnClient) LoadBalancerExists(lbName string) (bool, error) {
	lrp, err := c.GetLoadBalancer(lbName, true)
	return lrp != nil, err
}

// ListLoadBalancers list all load balancers
func (c *ovnClient) ListLoadBalancers() ([]ovnnb.LoadBalancer, error) {
	lbList := make([]ovnnb.LoadBalancer, 0)
	if err := c.ovnNbClient.WhereCache(func(lb *ovnnb.LoadBalancer) bool {
		// list all load balancers
		return true
	}).List(context.TODO(), &lbList); err != nil {
		return nil, fmt.Errorf("list load balancer: %v", err)
	}

	return lbList, nil
}

// LoadBalancerUpdateVips update load balancer vips
func (c *ovnClient) LoadBalancerUpdateVips(lbName string, vips map[string]string, op ovsdb.Mutator) error {
	if len(vips) == 0 {
		return fmt.Errorf("vips %s add or del to load balancer %s cannot be empty", vips, lbName)
	}

	mutation := func(lb *ovnnb.LoadBalancer) *model.Mutation {
		mutation := &model.Mutation{
			Field:   &lb.Vips,
			Value:   vips,
			Mutator: op,
		}

		return mutation
	}

	ops, err := c.LoadBalancerOp(lbName, mutation)
	if err != nil {
		return fmt.Errorf("generate operations for update load balancer %s vips %s: %v", lbName, vips, err)
	}

	if err := c.Transact("update-lb-vips", ops); err != nil {
		return fmt.Errorf("update vips %s for load balancer %s: %v", vips, lbName, err)
	}

	return nil
}

func (c *ovnClient) LoadBalancerOp(lbName string, mutationsFunc ...func(lb *ovnnb.LoadBalancer) *model.Mutation) ([]ovsdb.Operation, error) {
	lb, err := c.GetLoadBalancer(lbName, false)
	if err != nil {
		return nil, err
	}

	if len(mutationsFunc) == 0 {
		return nil, nil
	}

	mutations := make([]model.Mutation, 0, len(mutationsFunc))

	for _, f := range mutationsFunc {
		mutation := f(lb)

		if mutation != nil {
			mutations = append(mutations, *mutation)
		}
	}

	ops, err := c.ovnNbClient.Where(lb).Mutate(lb, mutations...)
	if err != nil {
		return nil, fmt.Errorf("generate operations for mutating load balancer %s: %v", lb.Name, err)
	}

	return ops, nil
}

// DeleteLoadBalancerOp create operation which delete load balancer
func (c *ovnClient) DeleteLoadBalancerOp(lbName string) ([]ovsdb.Operation, error) {
	lb, err := c.GetLoadBalancer(lbName, true)

	if err != nil {
		return nil, err
	}

	// not found, skip
	if lb == nil {
		return nil, nil
	}

	op, err := c.Where(lb).Delete()
	if err != nil {
		return nil, err
	}

	return op, nil
}
