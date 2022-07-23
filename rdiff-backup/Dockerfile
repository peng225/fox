FROM ubuntu:20.04

WORKDIR /root/
RUN apt-get update && \
    apt-get install -y rdiff-backup && \
    apt-get install -y ssh
ENTRYPOINT [ "rdiff-backup" ]
