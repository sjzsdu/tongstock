interface Section {
  type: 'header' | 'table' | 'paragraph' | 'link' | 'divider';
  content: string[][];
  text: string;
}

function parseTableLine(line: string): string[] {
  const cells: string[] = [];
  const parts = line.split('│');
  for (let i = 0; i < parts.length; i++) {
    const cell = parts[i].trim();
    if (i > 0 && i < parts.length - 1) {
      cells.push(cell);
    }
  }
  return cells;
}

function isTableBorder(line: string): boolean {
  return /^\s*[┌├└─┬┼┴┘┐]/.test(line) && line.includes('─');
}

export function parseTdxText(text: string): Section[] {
  const lines = text.replace(/\r/g, '').split('\n');
  const sections: Section[] = [];
  let currentTable: string[][] = [];
  let currentParagraph: string[] = [];

  const flushTable = () => {
    if (currentTable.length > 0) {
      sections.push({ type: 'table', content: currentTable, text: '' });
      currentTable = [];
    }
  };

  const flushParagraph = () => {
    if (currentParagraph.length > 0) {
      sections.push({ type: 'paragraph', content: [], text: currentParagraph.join('\n') });
      currentParagraph = [];
    }
  };

  for (const line of lines) {
    const trimmed = line.trim();

    if (trimmed === '') {
      flushTable();
      flushParagraph();
      continue;
    }

    if (isTableBorder(trimmed)) {
      flushParagraph();
      continue;
    }

    if (trimmed.startsWith('│')) {
      flushParagraph();
      const cells = parseTableLine(trimmed);
      if (cells.length > 0) {
        currentTable.push(cells);
      }
      continue;
    }

    flushTable();

    if (/^【.*】$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'header', content: [], text: trimmed });
      continue;
    }

    if (/^https?:\/\/\S+$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'link', content: [], text: trimmed });
      continue;
    }

    if (/^─+$/.test(trimmed)) {
      flushParagraph();
      sections.push({ type: 'divider', content: [], text: '' });
      continue;
    }

    if (/^[★☆◇●]/.test(trimmed) || trimmed.startsWith('    ') || trimmed.startsWith('│')) {
      currentParagraph.push(trimmed);
      continue;
    }

    currentParagraph.push(trimmed);
  }

  flushTable();
  flushParagraph();

  return sections;
}

export function renderTdxHtml(sections: Section[]): string {
  let html = '';

  for (const section of sections) {
    switch (section.type) {
      case 'header':
        html += `<h3 class="tdx-header">${escapeHtml(section.text)}</h3>`;
        break;

      case 'table':
        const maxCols = Math.max(...section.content.map(r => r.length));
        html += '<div class="tdx-table-wrap"><table class="tdx-table">';
        let isFirstRow = true;
        for (const row of section.content) {
          html += '<tr>';
          const isHeaderRow = isFirstRow || (row.length > 0 && /^●/.test(row[0]));
          for (let i = 0; i < maxCols; i++) {
            const cell = row[i] || '';
            const tag = isHeaderRow ? 'th' : 'td';
            html += `<${tag} class="${isHeaderRow ? 'tdx-th' : 'tdx-td'}">${escapeHtml(cell)}</${tag}>`;
          }
          html += '</tr>';
          isFirstRow = false;
        }
        html += '</table></div>';
        break;

      case 'paragraph':
        html += `<div class="tdx-para">${escapeHtml(section.text)}</div>`;
        break;

      case 'link':
        html += `<a href="${escapeHtml(section.text)}" target="_blank" class="tdx-link">${escapeHtml(section.text)}</a>`;
        break;

      case 'divider':
        html += '<hr class="tdx-divider" />';
        break;
    }
  }

  return html;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
