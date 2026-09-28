export function greeting(name: string): string {
  if (!name || name.trim().length === 0) {
    return 'Hello, stranger!'
  }
  return `Hello, ${name.trim()}!`
}
