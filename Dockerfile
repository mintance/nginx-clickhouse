FROM scratch

ADD nginx-clickhouse /

RUN mkdir /config

ADD /logs /logs
#ADD config.yaml /config/

CMD [ "/nginx-clickhouse" ]