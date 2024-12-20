export TF_VAR_project_name	 = tarhche
export TF_VAR_instance_name	 = backend
export TF_VAR_ssh_public_key = $(shell cat ssh-public-key.pub)
export EC2_SSH_ADDRESS		 =

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

public_key:
	ssh-keygen -y -f ssh-private-key.pem > ssh-public-key.pub

ssh:
	ssh -i "ssh-private-key.pem" ${EC2_SSH_ADDRESS}
