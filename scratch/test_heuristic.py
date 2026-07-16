import re

def test():
    txt_path = r"C:\Users\USER\Downloads\Holiness_Devotional_Extracted_New.txt"
    with open(txt_path, 'r', encoding='utf-8', errors='ignore') as f:
        lines = f.readlines()

    cleaned_lines = [line.strip() for line in lines if line.strip()]

    months = ['JANUARY', 'FEBRUARY', 'MARCH', 'APRIL', 'MAY', 'JUNE', 'JULY', 'AUGUST', 'SEPTEMBER', 'OCTOBER', 'NOVEMBER', 'DECEMBER', 'JAN', 'FEB', 'MAR', 'APR', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']
    months_pattern = r'^(' + '|'.join(months) + r')\s+\d{1,2}\b'

    section_patterns = [
        r'^PRAYER\b', r'^PRAYER POINT\b', r'^REFLECTION\b', r'^CONFESSION\b',
        r'^FURTHER READING\b', r'^MEMORY VERSE\b', r'^KEY POINT\b', r'^ACTION POINT\b',
        r'^DECLARATION\b'
    ]

    paragraphs = []
    current_p = []

    state = "normal" # "date", "title", "normal"

    for i, line in enumerate(cleaned_lines):
        next_line = cleaned_lines[i+1] if i+1 < len(cleaned_lines) else None
        
        current_p.append(line)
        
        # Check reasons to split
        split = False
        
        # Rule 1: Next line is Date
        if next_line and re.match(months_pattern, next_line, re.IGNORECASE) and len(next_line) < 20:
            split = True
            
        # Rule 2: Current line is Date
        elif re.match(months_pattern, line, re.IGNORECASE) and len(line) < 20:
            split = True
            state = "title"
            
        # Rule 3: Current line is Title
        elif state == "title":
            split = True
            state = "normal"
            
        # Rule 4: Next line is a section header
        elif next_line and any(re.match(pat, next_line, re.IGNORECASE) for pat in section_patterns) and len(next_line) < 35:
            split = True
            
        # Rule 5: Current line is a section header
        elif any(re.match(pat, line, re.IGNORECASE) for pat in section_patterns) and len(line) < 35:
            split = True
            
        # Rule 6: Current line ends with sentence ending and is short (< 55 chars)
        elif line[-1] in ['.', '?', '!', '"', ')'] and len(line) < 55:
            split = True
            
        # Rule 7: Current line is very short (< 30 chars)
        elif len(line) < 30:
            split = True
            
        if split:
            paragraphs.append(" ".join(current_p))
            current_p = []

    if current_p:
        paragraphs.append(" ".join(current_p))

    # Print paragraphs 15 to 30
    for idx, p in enumerate(paragraphs[15:30]):
        print(f"P{idx+16}: {p}\n")

if __name__ == "__main__":
    test()
