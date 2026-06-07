const PLATFORM_HOST = "tinycloud-platform.duckdns.org";

export function getPlatformAppUrl(name: string): string {
  return `https://${name}.${PLATFORM_HOST}/`;
}
