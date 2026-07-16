import fitz # PyMuPDF
import sys
import os

if len(sys.argv) < 3:
    print("Usage: python render_pdf.py <pdf_path> <output_dir>")
    sys.exit(1)

pdf_path = sys.argv[1]
output_dir = sys.argv[2]

if not os.path.exists(output_dir):
    os.makedirs(output_dir)

doc = fitz.open(pdf_path)
for i in range(len(doc)):
    page = doc[i]
    # Render page to image at 150 DPI (good balance of speed and OCR accuracy)
    pix = page.get_pixmap(dpi=150)
    pix.save(os.path.join(output_dir, f"page_{i+1:04d}.png"))
print(f"Successfully rendered {len(doc)} pages to {output_dir}")
