package osm

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	awsconst "kmodules.xyz/constants/aws"
	api "kmodules.xyz/objectstore-api/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewRcloneSecret creates a secret that contains the config file of Rclone.
// So, generally, if this secret is mounted in `etc/rclone`,
// the tree of `/etc/rclone` directory will be similar to,
//
// /etc/rclone
// └── config
func NewRcloneSecret(kc client.Client, name, namespace string, spec api.Backend, ownerReference []metav1.OwnerReference) (*core.Secret, error) {
	rcloneCtx, err := newContext(kc, spec, namespace)
	if err != nil {
		return nil, err
	}

	rcloneBytes := []byte(rcloneCtx)

	out := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerReference,
		},
		Data: map[string][]byte{
			"config": rcloneBytes,
		},
	}
	return out, nil
}

func newContext(kc client.Client, spec api.Backend, namespace string) (string, error) {
	config := make(map[string][]byte)
	if spec.StorageSecretName != "" {
		secret := &core.Secret{}
		err := kc.Get(context.TODO(), ktypes.NamespacedName{
			Name:      spec.StorageSecretName,
			Namespace: namespace,
		}, secret)
		if err != nil {
			return "", err
		}
		config = secret.Data
	}
	provider, err := spec.Provider()
	if err != nil {
		return "", err
	}

	if spec.S3 != nil {
		return cephContext(config, provider, spec), nil
	}
	if spec.Local != nil {
		return localContext(provider), nil
	}

	return "", fmt.Errorf("no storage provider is configured")
}

func cephContext(config map[string][]byte, provider string, spec api.Backend) string {
	keyID := config[awsconst.AWS_ACCESS_KEY_ID]
	key := config[awsconst.AWS_SECRET_ACCESS_KEY]

	return fmt.Sprintf(`[%s]
type = s3
provider = Ceph
env_auth = false
access_key_id = %s
secret_access_key = %s
region =
endpoint = %s
location_constraint =
acl =
server_side_encryption =
storage_class =
`, provider, keyID, key, spec.S3.Endpoint)
}

func localContext(provider string) string {
	return fmt.Sprintf(`[%s]
type = local
`, provider)
}
