package listener

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"

	api "k8s.io/api/core/v1"

	"github.com/coreos/alb-ingress-controller/pkg/alb/rules"
	"github.com/coreos/alb-ingress-controller/pkg/alb/targetgroups"
	albelbv2 "github.com/coreos/alb-ingress-controller/pkg/aws/elbv2"
	"github.com/coreos/alb-ingress-controller/pkg/util/log"
	util "github.com/coreos/alb-ingress-controller/pkg/util/types"
)

// Listener contains the relevant ID, Rules, and current/desired Listeners
type Listener struct {
	Current *elbv2.Listener
	Desired *elbv2.Listener
	Rules   rules.Rules
	Deleted bool
	logger  *log.Logger
}

type NewDesiredListenerOptions struct {
	Port           int64
	CertificateArn *string
	Logger         *log.Logger
}

type ReconcileOptions struct {
	Eventf          func(string, string, string, ...interface{})
	LoadBalancerArn *string
	TargetGroups    targetgroups.TargetGroups
}

// NewDesiredListener returns a new listener.Listener based on the parameters provided.
func NewDesiredListener(o *NewDesiredListenerOptions) *Listener {
	listener := &elbv2.Listener{
		Port:     aws.Int64(o.Port),
		Protocol: aws.String("HTTP"),
		DefaultActions: []*elbv2.Action{
			{
				Type: aws.String("forward"),
			},
		},
	}

	if o.CertificateArn != nil {
		listener.Certificates = []*elbv2.Certificate{
			{CertificateArn: o.CertificateArn},
		}
		listener.Protocol = aws.String("HTTPS")
	}

	listenerT := &Listener{
		Desired: listener,
		logger:  o.Logger,
	}

	return listenerT
}

// NewCurrentListener returns a new listener.Listener based on an elbv2.Listener.
func NewCurrentListener(listener *elbv2.Listener, logger *log.Logger) *Listener {
	listenerT := &Listener{
		Current: listener,
		logger:  logger,
	}

	return listenerT
}

// Reconcile compares the current and desired state of this Listener instance. Comparison
// results in no action, the creation, the deletion, or the modification of an AWS listener to
// satisfy the ingress's current state.
func (l *Listener) Reconcile(rOpts *ReconcileOptions) error {
	switch {

	case l.Desired == nil: // listener should be deleted
		if l.Current == nil {
			break
		}
		l.logger.Infof("Start Listener deletion.")
		if err := l.delete(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "DELETE", "%v listener deleted", *l.Current.Port)
		l.logger.Infof("Completed Listener deletion.")

	case l.Current == nil: // listener doesn't exist and should be created
		l.logger.Infof("Start Listener creation.")
		if err := l.create(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "CREATE", "%v listener created", *l.Current.Port)
		l.logger.Infof("Completed Listener creation. ARN: %s | Port: %v | Proto: %s.",
			*l.Current.ListenerArn, *l.Current.Port,
			*l.Current.Protocol)

	case l.NeedsModification(l.Desired): // current and desired diff; needs mod
		l.logger.Infof("Start Listener modification.")
		if err := l.modify(rOpts); err != nil {
			return err
		}
		rOpts.Eventf(api.EventTypeNormal, "MODIFY", "%v listener modified", *l.Current.Port)
		l.logger.Infof("Completed Listener modification. ARN: %s | Port: %s | Proto: %s.",
			*l.Current.ListenerArn, *l.Current.Port, *l.Current.Protocol)

	default:
		l.logger.Debugf("No listener modification required.")
	}

	return nil
}

// Adds a Listener to an existing ALB in AWS. This Listener maps the ALB to an existing TargetGroup.
func (l *Listener) create(rOpts *ReconcileOptions) error {
	l.Desired.LoadBalancerArn = rOpts.LoadBalancerArn

	// Set the listener default action to the targetgroup from the default rule.
	for _, rule := range l.Rules {
		if *rule.Desired.IsDefault {
			l.Desired.DefaultActions[0].TargetGroupArn = rule.TargetGroupArn(rOpts.TargetGroups)
		}
	}

	// Attempt listener creation.
	in := &elbv2.CreateListenerInput{
		Certificates:    l.Desired.Certificates,
		LoadBalancerArn: l.Desired.LoadBalancerArn,
		Protocol:        l.Desired.Protocol,
		Port:            l.Desired.Port,
		DefaultActions: []*elbv2.Action{
			{
				Type:           l.Desired.DefaultActions[0].Type,
				TargetGroupArn: l.Desired.DefaultActions[0].TargetGroupArn,
			},
		},
	}
	o, err := albelbv2.ELBV2svc.CreateListener(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error creating %v listener: %s", *l.Desired.Port, err.Error())
		l.logger.Errorf("Failed Listener creation: %s.", err.Error())
		return err
	}

	l.Current = o.Listeners[0]
	return nil
}

// Modifies a listener
// TODO: Determine if this needs to be implemented and if so, implement it.
func (l *Listener) modify(rOpts *ReconcileOptions) error {
	if l.Current == nil {
		// not a modify, a create
		return l.create(rOpts)
	}

	in := &elbv2.ModifyListenerInput{
		ListenerArn:    l.Current.ListenerArn,
		Certificates:   l.Desired.Certificates,
		Port:           l.Desired.Port,
		Protocol:       l.Desired.Protocol,
		SslPolicy:      l.Desired.SslPolicy,
		DefaultActions: l.Desired.DefaultActions,
	}

	o, err := albelbv2.ELBV2svc.ModifyListener(in)
	if err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error modifying %v listener: %s", *l.Desired.Port, err.Error())
		l.logger.Errorf("Failed Listener modification: %s.", err.Error())
	}
	l.Current = o.Listeners[0]

	return nil
}

// delete adds a Listener from an existing ALB in AWS.
func (l *Listener) delete(rOpts *ReconcileOptions) error {
	in := elbv2.DeleteListenerInput{
		ListenerArn: l.Current.ListenerArn,
	}

	if err := albelbv2.ELBV2svc.RemoveListener(in); err != nil {
		rOpts.Eventf(api.EventTypeWarning, "ERROR", "Error deleting %v listener: %s", *l.Current.Port, err.Error())
		l.logger.Errorf("Failed Listener deletion. ARN: %s: %s",
			*l.Current.ListenerArn, err.Error())
		return err
	}

	l.Deleted = true
	return nil
}

func (l *Listener) NeedsModification(target *elbv2.Listener) bool {
	switch {
	case l.Current == nil && l.Desired == nil:
		return false
	case l.Current == nil:
		return true
	case !util.DeepEqual(l.Current.Port, target.Port):
		return true
	case !util.DeepEqual(l.Current.Protocol, target.Protocol):
		return true
	case !util.DeepEqual(l.Current.Certificates, target.Certificates):
		return true
	}
	return false
}

// StripDesiredState removes the desired state from the listener.
func (l *Listener) StripDesiredState() {
	l.Desired = nil
	l.Rules.StripDesiredState()
}

// StripCurrentState removes the current state from the listener.
func (l *Listener) StripCurrentState() {
	l.Current = nil
	l.Rules.StripCurrentState()
}
