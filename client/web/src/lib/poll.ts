// Calls load now and then every `seconds`, until the returned stop is
// called. Use it inside $effect, which calls stop when its inputs change.
export function poll(load: () => void, seconds: number): () => void {
  load();
  const timer = setInterval(load, seconds * 1000);
  return () => clearInterval(timer);
}
