FROM debian:trixie-slim

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    HOME=/home/browser

# Portable Ungoogled Chromium. Debian does not ship a current package.
# https://github.com/ungoogled-software/ungoogled-chromium-portablelinux/releases/tag/153.0.8010.52-1
ARG UNGOOGLED_VERSION=153.0.8010.52-1
ARG UNGOOGLED_SHA256=49d01c59934c28d59caa30b3d283781bc32bfdc8b7f48bae831358526052a160

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        dbus \
        dbus-x11 \
        fonts-dejavu-core \
        fonts-liberation \
        iproute2 \
        libasound2t64 \
        libatk-bridge2.0-0t64 \
        libatk1.0-0t64 \
        libatspi2.0-0t64 \
        libcairo2 \
        libcups2t64 \
        libdav1d7 \
        libdbus-1-3 \
        libdouble-conversion3 \
        libegl1 \
        libexpat1 \
        libflac14 \
        libfontconfig1 \
        libfreetype6 \
        libgbm1 \
        libgl1 \
        libglib2.0-0t64 \
        libgtk-3-0t64 \
        libharfbuzz-subset0 \
        libharfbuzz0b \
        libjpeg62-turbo \
        liblcms2-2 \
        libminizip1t64 \
        libnspr4 \
        libnss3 \
        libnss3-tools \
        libopenjp2-7 \
        libopus0 \
        libpango-1.0-0 \
        libpulse0 \
        libx11-6 \
        libxcb1 \
        libxcomposite1 \
        libxdamage1 \
        libxext6 \
        libxfixes3 \
        libxkbcommon0 \
        libxnvctrl0 \
        libxrandr2 \
        unzip \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Linux instructions: https://status.tbank-online.com/certificates/#q7
# Both archives (root and issuing CAs) are copied into the system trust store,
# then registered with update-ca-certificates.
RUN mkdir -p /tmp/ru-certs /usr/local/share/ca-certificates/russian \
    && curl -fsSL -o /tmp/ru-certs/root.zip \
        https://gu-st.ru/content/lending/linux_russian_trusted_root_ca_pem.zip \
    && curl -fsSL -o /tmp/ru-certs/sub.zip \
        https://gu-st.ru/content/lending/russian_trusted_sub_ca_pem.zip \
    && unzip -j -o /tmp/ru-certs/root.zip -d /usr/local/share/ca-certificates/russian \
    && unzip -j -o /tmp/ru-certs/sub.zip -d /usr/local/share/ca-certificates/russian \
    && rm -rf /tmp/ru-certs \
    && update-ca-certificates

RUN arch="$(dpkg --print-architecture)" \
    && case "$arch" in \
        amd64) plat=x86_64 ;; \
        *) echo "ungoogled-chromium ${UNGOOGLED_VERSION} is published for amd64 only (got ${arch})" >&2; exit 1 ;; \
    esac \
    && curl -fsSL -o /tmp/ungoogled.tar.xz \
        "https://github.com/ungoogled-software/ungoogled-chromium-portablelinux/releases/download/${UNGOOGLED_VERSION}/ungoogled-chromium-${UNGOOGLED_VERSION}-${plat}_linux.tar.xz" \
    && echo "${UNGOOGLED_SHA256}  /tmp/ungoogled.tar.xz" | sha256sum -c - \
    && mkdir -p /opt/ungoogled-chromium \
    && tar -xJf /tmp/ungoogled.tar.xz -C /opt/ungoogled-chromium --strip-components=1 \
    && rm /tmp/ungoogled.tar.xz

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY safe-ru-bro /usr/local/bin/safe-ru-bro

# Window titles are stored as "$1 - Chromium" in the locale packs.
RUN chmod 755 /usr/local/bin/entrypoint.sh /usr/local/bin/safe-ru-bro \
    && /usr/local/bin/safe-ru-bro brand /opt/ungoogled-chromium/locales \
    && mkdir -p /home/browser/Downloads /tmp/pulse \
    && chmod 777 /home/browser /home/browser/Downloads /tmp/pulse

WORKDIR /home/browser
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
