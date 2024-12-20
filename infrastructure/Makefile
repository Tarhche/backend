export TF_VAR_PROJECT_NAME    = tarhche
export TF_VAR_INSTANCE_NAME   = backend
export TF_VAR_SSH_PUBLIC_KEY  = $(shell cat ssh-public-key.pub)
export TF_VAR_EC2_PRIVATE_KEY =

validate:
	terraform validate

init:
	terraform init

plan:
	terraform plan

apply:
	terraform apply

public_key:
	ssh-keygen -y -f ssh-private-key.pem > ssh-public-key.pub

ssh:
	ssh -i "ssh-private-key.pem" ${TF_VAR_EC2_PRIVATE_KEY}
