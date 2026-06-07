const APP_DOMAIN = "sasiru.lk";

export function getPlatformAppUrl(name: string): string {
  return `https://${name}.${APP_DOMAIN}/`;
}
