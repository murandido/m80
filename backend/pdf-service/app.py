from flask import Flask, Response, request
from weasyprint import HTML
from weasyprint.urls import URLFetcher

app = Flask(__name__)

# Every document the API sends (ticket, supplier and client reports) is self-contained:
# html/template output with inline SVG and system fonts, no external resource. So nothing
# needs fetching, and refusing every scheme but data: closes the whole SSRF/local-file
# class at the root - an <img src="file:///..."> or "http://169.254..." that slipped past
# escaping is dropped by WeasyPrint with a warning instead of being read.
url_fetcher = URLFetcher(allowed_protocols=["data"])


@app.route("/pdf", methods=["POST"])
def generate_pdf():
    html = request.get_data(as_text=True)
    if not html:
        return {"error": "No HTML provided"}, 400
    pdf_bytes = HTML(string=html, url_fetcher=url_fetcher).write_pdf()
    return Response(pdf_bytes, mimetype="application/pdf")


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000)
