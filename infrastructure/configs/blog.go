package configs

const (
	defaultBlogPort = 80

	// defaultRunnerManagerURL is where the runner manager sits on the local
	// stack, which is also what it is called in production.
	defaultRunnerManagerURL = "http://runner-manager:80"

	// defaultRunnerIngressDomain is the local one. Every *.localhost name
	// resolves to the loopback address, so a container is reachable in a
	// browser without touching any DNS.
	defaultRunnerIngressDomain = "runner.localhost:8021"
)

// Blog holds the configuration of the serve-blog command.
type Blog struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	WebURL     string `usage:"Absolute base URL the web frontend is reachable at, used to build the links sent by email." env:"WEB_URL" long:"web-url"`
	PrivateKey string `usage:"ECDSA private key, in PEM form, the authentication tokens are signed with." env:"PRIVATE_KEY" long:"private-key"`

	S3Endpoint   string `usage:"S3 compatible endpoint, as host:port." env:"S3_ENDPOINT" long:"s3-endpoint"`
	S3AccessKey  string `usage:"S3 access key." env:"S3_ACCESS_KEY" long:"s3-access-key"`
	S3SecretKey  string `usage:"S3 secret key." env:"S3_SECRET_KEY" long:"s3-secret-key"`
	S3BucketName string `usage:"S3 bucket uploads are stored in." env:"S3_BUCKET_NAME" long:"s3-bucket-name"`
	S3UseSSL     bool   `usage:"Whether the S3 endpoint is reached over TLS." env:"S3_USE_SSL" long:"s3-use-ssl"`

	MailFrom     string `usage:"Address outgoing mail is sent from." env:"MAIL_SMTP_FROM" long:"mail-smtp-from"`
	MailHost     string `usage:"SMTP host outgoing mail is relayed through." env:"MAIL_SMTP_HOST" long:"mail-smtp-host"`
	MailPort     string `usage:"SMTP port." env:"MAIL_SMTP_PORT" long:"mail-smtp-port"`
	MailUsername string `usage:"SMTP user, when the relay authenticates." env:"MAIL_SMTP_USERNAME" long:"mail-smtp-username"`
	MailPassword string `usage:"SMTP password, when the relay authenticates." env:"MAIL_SMTP_PASSWORD" long:"mail-smtp-password"`

	RunnerManagerURL    string `usage:"Base URL of the runner manager's API, which the dashboard passes container and stack commands to." env:"RUNNER_MANAGER_URL" long:"runner-manager-url"`
	RunnerIngressDomain string `usage:"Domain a runner container's exposed ports are served on, used to build the addresses the dashboard shows." env:"RUNNER_INGRESS_DOMAIN" long:"runner-ingress-domain"`
}

// NewBlog returns the configuration of the serve-blog command, holding the
// defaults it runs with until the console overrides them. The command owns the
// struct it is given, so nothing it parses reaches another command.
func NewBlog() *Blog {
	return &Blog{
		Port:                defaultBlogPort,
		RunnerManagerURL:    defaultRunnerManagerURL,
		RunnerIngressDomain: defaultRunnerIngressDomain,
	}
}
