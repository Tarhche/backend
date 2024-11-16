export function generateRange(size: number, from: number = 0) {
  return new Array(size).fill(1).map((_, i) => i + from + 1);
}
