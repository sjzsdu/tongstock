type StreamEvent = {
  type: string;
  delta?: string;
  error?: string;
  code?: string;
  message?: string;
  request_id?: string;
};

export async function readSSE(
  res: Response,
  onEvent: (event: StreamEvent) => void,
) {
  if (!res.body) throw new Error('stream response body missing');
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let idx = -1;
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      for (const line of raw.split('\n')) {
        if (!line.startsWith('data:')) continue;
        const json = line.slice(5).trim();
        if (!json) continue;
        try {
          onEvent(JSON.parse(json));
        } catch {
          // Skip malformed SSE lines
        }
      }
    }
  }
}
