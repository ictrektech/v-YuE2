ARG RUNTIME_BASE=ubuntu:22.04
FROM ${RUNTIME_BASE}
ARG APP_VERSION
ENV APP_VERSION=${APP_VERSION} YUE_STUDIO_BIN=/opt/v-yue2/music-server YUE_WEB_ROOT=/opt/v-yue2/web YUE_ENGINE_ROOT=/opt/v-yue2/engine YUE_ENGINE_BIN=/opt/v-yue2/engine/yue-server YUE_VOS=1 DO_NOT_TRACK=1 STUDIO_TELEMETRY=0
ENV LD_LIBRARY_PATH=/opt/v-yue2/engine
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgomp1 libstdc++6 libdbus-1-3 libssl3 && rm -rf /var/lib/apt/lists/*
COPY bin/ /opt/v-yue2/
COPY engine/ /opt/v-yue2/engine/
COPY web/ /opt/v-yue2/web/
COPY notices/ /opt/v-yue2/notices/
VOLUME /data
EXPOSE 8791
HEALTHCHECK --interval=30s --timeout=5s CMD ["/opt/v-yue2/v-yue2-gateway", "healthcheck"]
ENTRYPOINT ["/opt/v-yue2/v-yue2-gateway"]
