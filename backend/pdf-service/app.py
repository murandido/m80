from flask import Flask, request, Response
from weasyprint import HTML

app = Flask(__name__)


@app.route("/pdf", methods=["POST"])
def generate_pdf():
    html = request.get_data(as_text=True)
    if not html:
        return {"error": "No HTML provided"}, 400
    pdf_bytes = HTML(string=html).write_pdf()
    return Response(pdf_bytes, mimetype="application/pdf")


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000)
