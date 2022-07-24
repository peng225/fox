FROM ubuntu:20.04

WORKDIR /root/
RUN apt-get update && \
    apt-get install -y rdiff-backup && \
    apt-get install -y ssh
RUN yes | ssh-keygen -q -t rsa -N '' -f ~/.ssh/id_rsa
RUN cat .ssh/id_rsa.pub > .ssh/authorized_keys

EXPOSE 22
ENTRYPOINT [ "rdiff-backup" ]
