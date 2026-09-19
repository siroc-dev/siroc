FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    container=docker \
    PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

STOPSIGNAL SIGRTMIN+3

RUN apt-get update && apt-get install -y --no-install-recommends \
        systemd systemd-sysv sudo curl ca-certificates gnupg git \
        software-properties-common apt-transport-https \
        build-essential xz-utils unzip \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && curl -fsSL https://go.dev/dl/go1.24.2.linux-amd64.tar.gz | tar -C /usr/local -xz \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN find /etc/systemd/system /lib/systemd/system -path '*.wants/*' \
        -not -name '*journald*' \
        -not -name '*systemd-tmpfiles*' \
        -not -name '*systemd-user-sessions*' \
        -exec rm -f {} + \
    && systemctl mask \
        systemd-resolved.service \
        systemd-networkd-wait-online.service \
        systemd-firstboot.service \
        udev.service \
        udev.socket

COPY docker/siroc-bootstrap.service /etc/systemd/system/siroc-bootstrap.service
COPY docker/siroc-agent.service /etc/systemd/system/siroc-agent.service
COPY docker/siroc-panel.service /etc/systemd/system/siroc-panel.service
RUN chmod 644 /etc/systemd/system/siroc-*.service && systemctl enable siroc-bootstrap.service

COPY . /opt/siroc
RUN find /opt/siroc/scripts /opt/siroc/docker -type f \( -name '*.sh' -o -name '*.service' \) -exec sed -i 's/\r$//' {} + \
    && chmod +x /opt/siroc/scripts/*.sh

WORKDIR /opt/siroc
VOLUME ["/sys/fs/cgroup"]
CMD ["/lib/systemd/systemd"]
