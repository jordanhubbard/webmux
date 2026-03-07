export const argon2id = 2;
export async function hash(password: string): Promise<string> {
  return `$argon2id$mock$${Buffer.from(password).toString('base64')}`;
}
export async function verify(hash: string, password: string): Promise<boolean> {
  return hash === `$argon2id$mock$${Buffer.from(password).toString('base64')}`;
}
export default { argon2id, hash, verify };
