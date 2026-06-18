run:
	go run . 
push:
	git add . && git commit -m "$(m)" && git push
docker:
	#Pass version using v variable
	sudo docker build  --platform linux/amd64 -t dafraer/ttc-tg-bot:$(v) .
	docker push dafraer/tts-tg-bot:$(v)