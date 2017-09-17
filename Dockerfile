FROM scratch

ADD nginx-clickhouse /

ADD /logs /logs
ADD config.yaml /config/

CMD [ "/nginx-clickhouse" ]