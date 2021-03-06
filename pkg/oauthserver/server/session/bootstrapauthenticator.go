package session

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/origin/pkg/oauthserver/authenticator/password/bootstrap"
)

func NewBootstrapAuthenticator(delegate SessionAuthenticator, secrets v1.SecretsGetter, namespaces v1.NamespacesGetter, store Store) SessionAuthenticator {
	return &bootstrapAuthenticator{
		delegate:   delegate,
		secrets:    secrets.Secrets(metav1.NamespaceSystem),
		namespaces: namespaces.Namespaces(),
		store:      store,
	}
}

type bootstrapAuthenticator struct {
	delegate   SessionAuthenticator
	secrets    v1.SecretInterface
	namespaces v1.NamespaceInterface
	store      Store
}

func (b *bootstrapAuthenticator) AuthenticateRequest(req *http.Request) (user.Info, bool, error) {
	u, ok, err := b.delegate.AuthenticateRequest(req)
	if err != nil || !ok || u.GetName() != bootstrap.BootstrapUser {
		return u, ok, err
	}

	// make sure that the password has not changed since this cookie was issued
	// note that this is not really for security - it is so that we do not annoy the user
	// by letting them log in successfully only to have a token that does not work
	_, uid, ok, err := bootstrap.HashAndUID(b.secrets, b.namespaces)
	if err != nil || !ok {
		return nil, ok, err
	}
	if uid != u.GetUID() {
		return nil, false, nil
	}

	return u, true, nil
}

func (b *bootstrapAuthenticator) AuthenticationSucceeded(user user.Info, state string, w http.ResponseWriter, req *http.Request) (bool, error) {
	if user.GetName() != bootstrap.BootstrapUser {
		return b.delegate.AuthenticationSucceeded(user, state, w, req)
	}

	// since osin is the IDP for this user, we increase the length
	// of the session to allow for transitions between components
	// this means the user could stay authenticated for one hour + OAuth access token lifetime
	return false, putUser(b.store, w, user, time.Hour)
}

func (b *bootstrapAuthenticator) InvalidateAuthentication(w http.ResponseWriter, user user.Info) error {
	if user.GetName() != bootstrap.BootstrapUser {
		return b.delegate.InvalidateAuthentication(w, user)
	}

	// the IDP is responsible for maintaining the user's session
	// since osin is the IDP for the bootstrap user, we do not invalidate its session
	// this is safe to do because we tie the cookie and token to the password hash
	return nil
}
