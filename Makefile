ps:
	podman compose ps

up:
	podman compose up --build -d

down:
	podman compose down --remove-orphans --volumes

sh%:
	podman compose exec -it $* sh

