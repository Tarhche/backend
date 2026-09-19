package configs

const (
	defaultBlogPort = 80

	// defaultRunnerManagerURL is where the runner manager sits on the local
	// stack, which is also what it is called in production.
	defaultRunnerManagerURL = "http://runner-manager:80"

	// defaultRunnerPublicIngressDomain is where a browser reaches a task,
	// which is the ingress's own domain with the port it is published on — the
	// ingress matches the hostname alone, so its copy carries no port. Every
	// *.localhost name resolves to the loopback address, so a task is
	// reachable without touching any DNS.
	defaultRunnerPublicIngressDomain = "runner.localhost:8030"
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

	GoogleClientID       string `usage:"OAuth client id signing people in with their Google account. Empty leaves Google unoffered." env:"OAUTH_GOOGLE_CLIENT_ID" long:"oauth-google-client-id"`
	GoogleClientSecret   string `usage:"OAuth client secret for Google." env:"OAUTH_GOOGLE_CLIENT_SECRET" long:"oauth-google-client-secret"`
	GoogleRedirectURL    string `usage:"Address Google sends the browser back to, which must be one it was registered with." env:"OAUTH_GOOGLE_REDIRECT_URL" long:"oauth-google-redirect-url"`
	GithubClientID       string `usage:"OAuth client id signing people in with their GitHub account. Empty leaves GitHub unoffered." env:"OAUTH_GITHUB_CLIENT_ID" long:"oauth-github-client-id"`
	GithubClientSecret   string `usage:"OAuth client secret for GitHub." env:"OAUTH_GITHUB_CLIENT_SECRET" long:"oauth-github-client-secret"`
	GithubRedirectURL    string `usage:"Address GitHub sends the browser back to, which must be one it was registered with." env:"OAUTH_GITHUB_REDIRECT_URL" long:"oauth-github-redirect-url"`
	LinkedinClientID     string `usage:"OAuth client id signing people in with their LinkedIn account. Empty leaves LinkedIn unoffered." env:"OAUTH_LINKEDIN_CLIENT_ID" long:"oauth-linkedin-client-id"`
	LinkedinClientSecret string `usage:"OAuth client secret for LinkedIn." env:"OAUTH_LINKEDIN_CLIENT_SECRET" long:"oauth-linkedin-client-secret"`
	LinkedinRedirectURL  string `usage:"Address LinkedIn sends the browser back to, which must be one it was registered with." env:"OAUTH_LINKEDIN_REDIRECT_URL" long:"oauth-linkedin-redirect-url"`

	RunnerManagerURL    string `usage:"Base URL of the runner manager's API, which the dashboard passes task and stack commands to." env:"RUNNER_MANAGER_URL" long:"runner-manager-url"`
	RunnerIngressDomain string `usage:"Domain a runner task's exposed ports are served on, used to build the addresses the dashboard shows." env:"RUNNER_INGRESS_DOMAIN" long:"runner-ingress-domain"`
}

// NewBlog returns the configuration of the serve-blog command, holding the
// defaults it runs with until the console overrides them. The command owns the
// struct it is given, so nothing it parses reaches another command.
func NewBlog() *Blog {
	return &Blog{
		Port:                defaultBlogPort,
		RunnerManagerURL:    defaultRunnerManagerURL,
		RunnerIngressDomain: defaultRunnerPublicIngressDomain,
	}
}
