FROM python:3.9
WORKDIR /soundmate
COPY . .
RUN pip install --upgrade pip
RUN pip install -e .
CMD ["/bin/bash", "-c", "python3.9 runner.py"]