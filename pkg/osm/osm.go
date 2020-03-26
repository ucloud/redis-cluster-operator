package osm

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	awsconst "kmodules.xyz/constants/aws"
	azconst "kmodules.xyz/constants/azure"
	googconst "kmodules.xyz/constants/google"
	osconst "kmodules.xyz/constants/openstack"

	stringz "github.com/appscode/go/strings"
	"github.com/appscode/go/types"
	otx "github.com/appscode/osm/context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	_s3 "github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"gomodules.xyz/stow"
	"gomodules.xyz/stow/azure"
	gcs "gomodules.xyz/stow/google"
	"gomodules.xyz/stow/local"
	"gomodules.xyz/stow/s3"
	"gomodules.xyz/stow/swift"
	core "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	api "kmodules.xyz/objectstore-api/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SecretMountPath = "/etc/rclone"
	CaCertFileName  = "ca.crt"
)

func CheckBucketAccess(client client.Client, spec api.Backend, namespace string) error {
	cfg, err := NewOSMContext(client, spec, namespace)
	if err != nil {
		return err
	}
	loc, err := stow.Dial(cfg.Provider, cfg.Config)
	if err != nil {
		return err
	}
	bucket, err := spec.Container()
	if err != nil {
		return err
	}
	c, err := loc.Container(bucket)
	if err != nil {
		return err
	}
	return c.HasWriteAccess()
}

func NewOSMContext(client client.Client, spec api.Backend, namespace string) (*otx.Context, error) {
	config := make(map[string][]byte)

	if spec.StorageSecretName != "" {
		secret := &core.Secret{}
		err := client.Get(context.TODO(), ktypes.NamespacedName{
			Name:      spec.StorageSecretName,
			Namespace: namespace,
		}, secret)
		if err != nil {
			return nil, err
		}
		config = secret.Data
	}

	nc := &otx.Context{
		Name:   "objectstore",
		Config: stow.ConfigMap{},
	}

	if spec.S3 != nil {
		nc.Provider = s3.Kind

		keyID, foundKeyID := config[awsconst.AWS_ACCESS_KEY_ID]
		key, foundKey := config[awsconst.AWS_SECRET_ACCESS_KEY]
		if foundKey && foundKeyID {
			nc.Config[s3.ConfigAccessKeyID] = string(keyID)
			nc.Config[s3.ConfigSecretKey] = string(key)
			nc.Config[s3.ConfigAuthType] = "accesskey"
		} else {
			nc.Config[s3.ConfigAuthType] = "iam"
		}
		if spec.S3.Endpoint == "" || strings.HasSuffix(spec.S3.Endpoint, ".amazonaws.com") {
			// Using s3 and not s3-compatible service like minio or rook, etc. Now, find region
			var sess *session.Session
			var err error
			if nc.Config[s3.ConfigAuthType] == "iam" {
				// The aws sdk does not currently support automatically setting the region based on an instances placement.
				// This automatically sets region based on ec2 instance metadata when running on EC2.
				// ref: https://docs.aws.amazon.com/sdk-for-javascript/v2/developer-guide/setting-region.html#setting-region-order-of-precedence
				var c aws.Config
				if s, e := session.NewSession(); e == nil {
					if region, e := ec2metadata.New(s).Region(); e == nil {
						c.WithRegion(region)
					}
				}
				sess, err = session.NewSessionWithOptions(session.Options{
					Config: c,
					// Support MFA when authing using assumed roles.
					SharedConfigState:       session.SharedConfigEnable,
					AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
				})
			} else {
				sess, err = session.NewSessionWithOptions(session.Options{
					Config: aws.Config{
						Credentials: credentials.NewStaticCredentials(string(keyID), string(key), ""),
						Region:      aws.String("us-east-1"),
					},
					// Support MFA when authing using assumed roles.
					SharedConfigState:       session.SharedConfigEnable,
					AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
				})
			}
			if err != nil {
				return nil, err
			}
			svc := _s3.New(sess)
			out, err := svc.GetBucketLocation(&_s3.GetBucketLocationInput{
				Bucket: types.StringP(spec.S3.Bucket),
			})
			if err != nil {
				return nil, err
			}
			nc.Config[s3.ConfigRegion] = stringz.Val(types.String(out.LocationConstraint), "us-east-1")
		} else {
			nc.Config[s3.ConfigEndpoint] = spec.S3.Endpoint
			u, err := url.Parse(spec.S3.Endpoint)
			if err != nil {
				return nil, err
			}
			nc.Config[s3.ConfigDisableSSL] = strconv.FormatBool(u.Scheme == "http")

			cacertData, ok := config[awsconst.CA_CERT_DATA]
			if ok && u.Scheme == "https" {
				nc.Config[s3.ConfigCACertData] = string(cacertData)
			}
		}
		return nc, nil
	} else if spec.GCS != nil {
		nc.Provider = gcs.Kind
		nc.Config[gcs.ConfigProjectId] = string(config[googconst.GOOGLE_PROJECT_ID])
		nc.Config[gcs.ConfigJSON] = string(config[googconst.GOOGLE_SERVICE_ACCOUNT_JSON_KEY])
		return nc, nil
	} else if spec.Azure != nil {
		nc.Provider = azure.Kind
		nc.Config[azure.ConfigAccount] = string(config[azconst.AZURE_ACCOUNT_NAME])
		nc.Config[azure.ConfigKey] = string(config[azconst.AZURE_ACCOUNT_KEY])
		return nc, nil
	} else if spec.Local != nil {
		nc.Provider = local.Kind
		nc.Config[local.ConfigKeyPath] = spec.Local.MountPath
		return nc, nil
	} else if spec.Swift != nil {
		nc.Provider = swift.Kind
		// https://github.com/restic/restic/blob/master/src/restic/backend/swift/config.go
		for _, val := range []struct {
			stowKey   string
			secretKey string
		}{
			// v2/v3 specific
			{swift.ConfigUsername, osconst.OS_USERNAME},
			{swift.ConfigKey, osconst.OS_PASSWORD},
			{swift.ConfigRegion, osconst.OS_REGION_NAME},
			{swift.ConfigTenantAuthURL, osconst.OS_AUTH_URL},

			// v3 specific
			{swift.ConfigDomain, osconst.OS_USER_DOMAIN_NAME},
			{swift.ConfigTenantName, osconst.OS_PROJECT_NAME},
			{swift.ConfigTenantDomain, osconst.OS_PROJECT_DOMAIN_NAME},

			// v2 specific
			{swift.ConfigTenantId, osconst.OS_TENANT_ID},
			{swift.ConfigTenantName, osconst.OS_TENANT_NAME},

			// v1 specific
			{swift.ConfigTenantAuthURL, osconst.ST_AUTH},
			{swift.ConfigUsername, osconst.ST_USER},
			{swift.ConfigKey, osconst.ST_KEY},

			// Manual authentication
			{swift.ConfigStorageURL, osconst.OS_STORAGE_URL},
			{swift.ConfigAuthToken, osconst.OS_AUTH_TOKEN},
		} {
			if _, exists := nc.Config.Config(val.stowKey); !exists {
				nc.Config[val.stowKey] = string(config[val.secretKey])
			}
		}
		return nc, nil
	}
	return nil, errors.New("no storage provider is configured")
}
