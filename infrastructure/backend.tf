provider "aws" {
  region = "eu-central-1"
}

variable "project_name" {
  description = "Project tag given to each deployed Instance"
  type = string 
}

variable "instance_name" {
  description = "instance_name"
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key"
  type        = string
}

resource "aws_security_group" "backend" {
  name        = var.instance_name
  description = "Allow HTTP, HTTPS, and SSH inbound traffic"

	tags = {
    project_name = var.project_name
	}

  # Allow SSH (port 22) from any IP address
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Allow HTTP (port 80) from any IP address
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]  # Allow HTTP from anywhere
  }

  # Allow HTTPS (port 443) from any IP address
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"  # all protocols
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_key_pair" "ssh_public_key" {
  key_name   = var.instance_name
  public_key = var.ssh_public_key

	tags = {
    project_name = var.project_name
	}
}

resource "aws_instance" "backend" {
  ami           							= "ami-0e54671bdf3c8ed8d"  # Amazon linux 2023
  instance_type 							= "t2.micro"
  key_name 										= aws_key_pair.ssh_public_key.key_name

  root_block_device {
    delete_on_termination = true
    encrypted = false
    volume_size = 15
    volume_type = "gp3"

    tags = {
      project_name = var.project_name
	  }
  }

  security_groups = [
		aws_security_group.backend.name
	]

	tags = {
    project_name = var.project_name
	}
}

resource "aws_eip" "backend" {
  instance = aws_instance.backend.id
  domain   = "vpc"

	tags = {
    project_name = var.project_name
	}
}
