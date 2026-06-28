export type PublicConfig = {
  appName: string;
  environment: string;
  startingCashCents: number;
};

export async function fetchPublicConfig(): Promise<PublicConfig> {
  const response = await fetch('/api/config', {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Config request failed with ${response.status}`);
  }

  return response.json() as Promise<PublicConfig>;
}
