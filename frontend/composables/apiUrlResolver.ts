function baseUrl(): string {
    const publicBaseURL = 'http://127.0.0.1:8000/';
    const internalBaseURL = 'http://app/';

    let url = process.client ? publicBaseURL : internalBaseURL;

    return url.replace(/\/$/, "");
}

export function useApiUrlResolver() {
    const url = baseUrl();

    return {
        resolve: (path: string): string => `${url}/${path}`
    }
}