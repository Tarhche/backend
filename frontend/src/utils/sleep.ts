export function waitFor(ms: number) {
  return new Promise((res) => {
    setTimeout(res, ms);
  });
}
