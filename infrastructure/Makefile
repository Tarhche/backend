export TF_VAR_project_name	 = tarhche
export TF_VAR_instance_name	 = backend

export EC2_SSH_ADDRESS		 = ec2-3-124-72-48.eu-central-1.compute.amazonaws.com
export EC2_SSH_USER 		 = ubuntu
export EC2_SSH_ENDPOINT 	 = ${EC2_SSH_USER}@${EC2_SSH_ADDRESS}
export VOLUME_PATH           = ./tmp/volume_01

export MONGO_USERNAME = test
export MONGO_PASSWORD = test‍

export DASHBOARD_MONGO_USERNAME    = username
export DASHBOARD_MONGO_PASSWORD    = password
export DASHBOARD_MONGO_MONGODB_URL = mongodb://${MONGO_USERNAME}:${MONGO_PASSWORD}@mongodb:27017

export BACKEND_NATS_URL =
export BACKEND_PRIVATE_KEY =

export BACKEND_MONGO_HOST          = mongodb
export BACKEND_MONGO_PORT          = 27017
export BACKEND_MONGO_SCHEME        = mongodb
export BACKEND_MONGO_DATABASE_NAME = test
export BACKEND_MONGO_USERNAME      = ${MONGO_USERNAME}
export BACKEND_MONGO_PASSWORD      = ${MONGO_PASSWORD}

export BACKEND_MAIL_SMTP_PASSWORD =
export BACKEND_MAIL_SMTP_HOST     =
export BACKEND_MAIL_SMTP_FROM     =
export BACKEND_MAIL_SMTP_USERNAME =
export BACKEND_MAIL_SMTP_PORT     =

export BACKEND_S3_ENDPOINT    =
export BACKEND_S3_SECRET_KEY  =
export BACKEND_S3_ACCESS_KEY  =
export BACKEND_S3_USE_SSL     = false
export BACKEND_S3_BUCKET_NAME =

export PROXY_IMAGE = ghcr.io/tarhche/proxy:latest

export APP_IMAGE = ghcr.io/tarhche/backend:latest

# username: admin
# password: admin-password (in bcrypt, a dollar-sign should be escaped by an arbitrary dollar-sign ($ --> $$))
export PORTAINER_ADMIN_PASSWORD = $$2a$$12$$4xcOa82Ni5rjgQF.v.JWi.i71OyUm3fwmfWiumgJHIAPGU.uOw3qu

export FRONTEND_IMAGE                        = ghcr.io/tarhche/frontend:latest
export NEXT_PUBLIC_EXTERNAL_BACKEND_BASE_URL =
export INTERNAL_BACKEND_BASE_URL             = http://app
export NEXT_PUBLIC_FILES_BASE_URL            =

validate:
	terraform validate

fmt: 
	terraform fmt

init:
	terraform init

state:
	terraform state list

plan:
	terraform plan

apply:
	terraform apply
	rm -f terraform.tfstate *.tfstate.*

public_key:
	ssh-keygen -y -f ssh-private-key.pem > ssh-public-key.pub

ssh:
	ssh -i "ssh-private-key.pem" ${EC2_SSH_ENDPOINT}

up:
	docker compose -f compose.mongodb.yaml --project-name mongodb up --pull always --detach
	docker compose -f compose.mongodb_dashboard.yaml --project-name mongodb_dashboard up --pull always --detach
	docker compose -f compose.nats.yaml --project-name nats up --pull always --detach
	docker compose -f compose.docker.yaml --project-name docker up --pull always --detach
	docker compose -f compose.docker_dashboard.yaml --project-name docker_dashboard up --pull always --detach
	docker compose -f compose.app.yaml --project-name app up --pull always --detach
	docker compose -f compose.frontend.yaml --project-name frontend up --pull always --detach
	docker compose -f compose.proxy.yaml --project-name proxy up --pull always --detach

down:
	docker compose -f compose.frontend.yaml --project-name frontend down --volumes --remove-orphans
	docker compose -f compose.app.yaml --project-name app down --volumes --remove-orphans
	docker compose -f compose.proxy.yaml --project-name proxy down --volumes --remove-orphans
	docker compose -f compose.nats.yaml --project-name nats down --volumes --remove-orphans
	docker compose -f compose.docker_dashboard.yaml --project-name docker_dashboard down --volumes --remove-orphans
	docker compose -f compose.docker.yaml --project-name docker down --volumes --remove-orphans
	docker compose -f compose.mongodb_dashboard.yaml --project-name mongodb_dashboard down --volumes --remove-orphans
	docker compose -f compose.mongodb.yaml --project-name mongodb down --volumes --remove-orphans
