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
	docker stack deploy -c compose.mongodb.yaml mongodb --detach=false
	docker stack deploy -c compose.mongodb_dashboard.yaml mongodb_dashboard --detach=false
	docker stack deploy -c compose.nats.yaml nats --detach=false
	docker stack deploy -c compose.docker.yaml docker --detach=false
	docker stack deploy -c compose.docker_dashboard.yaml docker_dashboard --detach=false
	docker stack deploy -c compose.backend.yaml backend --detach=false
	docker stack deploy -c compose.frontend.yaml frontend --detach=false
	docker stack deploy -c compose.proxy.yaml proxy --detach=false

down:
	docker stack rm proxy
	docker stack rm frontend
	docker stack rm backend
	docker stack rm docker_dashboard
	docker stack rm docker
	docker stack rm nats
	docker stack rm mongodb_dashboard
	docker stack rm mongodb
