from boxer import BoxerClient

with BoxerClient() as client:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "print('hello world')"],
    )
    print(result.stdout)
