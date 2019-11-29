package osm

import (
	"context"
	"fmt"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	api "kmodules.xyz/objectstore-api/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCephSecret(kc client.Client, name, namespace string, spec api.Backend) (*core.Secret, error) {
	if spec.S3 == nil {
		return nil, fmt.Errorf("only suport ceph s3")
	}

	config := make(map[string][]byte)
	if spec.StorageSecretName != "" {
		secret := &core.Secret{}
		err := kc.Get(context.TODO(), ktypes.NamespacedName{
			Name:      spec.StorageSecretName,
			Namespace: namespace,
		}, secret)
		if err != nil {
			return nil, err
		}
		config = secret.Data
	}

	keyID := config[api.AWS_ACCESS_KEY_ID]
	key := config[api.AWS_SECRET_ACCESS_KEY]

	osmBytes := fmt.Sprintf(`[ceph]
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
storage_class =`, keyID, key, spec.S3.Endpoint)
	out := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"config": []byte(osmBytes),
		},
	}
	return out, nil
}
