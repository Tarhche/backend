export TF_VAR_project_name	 = tarhche
export TF_VAR_instance_name	 = backend

export EC2_SSH_ADDRESS		 =
export EC2_SSH_USER 		 =
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

export APP_IMAGE = ghcr.io/tarhche/backend:latest

# username: admin
# password: admin-password (in bcrypt, a dollar-sign should be escaped by an arbitrary dollar-sign ($ --> $$))
export PORTAINER_ADMIN_PASSWORD = $$2a$$12$$4xcOa82Ni5rjgQF.v.JWi.i71OyUm3fwmfWiumgJHIAPGU.uOw3qu

export FRONTEND_IMAGE                        = ghcr.io/tarhche/frontend:latest
export NEXT_PUBLIC_EXTERNAL_BACKEND_BASE_URL =
export INTERNAL_BACKEND_BASE_URL             =
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
	docker compose \
		-f compose.mongodb.yaml \
		-f compose.nats.yaml \
		-f compose.docker.yaml \
		-f compose.backend.yaml \
		-f compose.frontend.yaml \
		-f compose.proxy.yaml \
		up --detach --pull always

down:
	docker compose \
		-f compose.mongodb.yaml \
		-f compose.nats.yaml \
		-f compose.docker.yaml \
		-f compose.backend.yaml \
		-f compose.frontend.yaml \
		-f compose.proxy.yaml \
		down --volumes --remove-orphans

ps:
	docker compose \
		-f compose.mongodb.yaml \
		-f compose.nats.yaml \
		-f compose.docker.yaml \
		-f compose.backend.yaml \
		-f compose.frontend.yaml \
		-f compose.proxy.yaml \
		ps -a

logs%:
	docker compose \
		-f compose.mongodb.yaml \
		-f compose.nats.yaml \
		-f compose.docker.yaml \
		-f compose.backend.yaml \
		-f compose.frontend.yaml \
		-f compose.proxy.yaml \
		logs $*
