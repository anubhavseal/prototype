import os
import time
import threading
from flask import Flask, Response, render_template

app = Flask(__name__)

DATASETS_LOGS = "./data"
os.makedirs(DATASETS_LOGS, exist_ok=True)

def mock_deployment(deployment_id: str):
    filepath = os.path.join(DATASETS_LOGS, f"{deployment_id}.log")
    with open(filepath, "a", encoding="utf-8") as fp:
        for i in range(100000):
            fp.write(f"{time.strftime('%Y-%m-%d %H:%M:%S')} Fake log line {i}\n")
            fp.flush()
            time.sleep(0.2)

@app.route("/")
def index_handler():
    deployments=[x.split(".")[0] for x in os.listdir(DATASETS_LOGS) if x.endswith(".log")]
    return render_template("index.html", deployments=deployments)

@app.route("/deployments/<deployment_id>")
def deployment_handler(deployment_id):
    return render_template("deployment.html", deployment_id=deployment_id)

@app.route("/deployments", methods=["POST"])
def new_deployment():
    deployment_id=str(int(time.time()*1000000))
    t=threading.Thread(target=mock_deployment, args=(deployment_id,), daemon=True)
    t.start()
    return deployment_id,200

@app.route("/deployments/<deployment_id>/stream")
def stream_logs(deployment_id):
    filepath=os.path.join(DATASETS_LOGS, f"{deployment_id}.log")
    def event_stream():
        with open(filepath, "r") as f:
            while True:
                line=f.readline()
                if line:
                    yield f"data: {line.strip()}\n\n"
                time.sleep(0.1)
    return Response(event_stream(), mimetype="text/event-stream")

if __name__ == "__main__":
    app.run(debug=True, port=5000, threaded=True)
