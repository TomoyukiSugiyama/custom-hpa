FROM debian
COPY ./custom-hpa /custom-hpa
ENTRYPOINT /custom-hpa