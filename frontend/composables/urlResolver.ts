function baseUrl(): string {
    const publicBaseURL = 'http://127.0.0.1:8000/';
    const internalBaseURL = 'http://app/';

    let url = process.client ? publicBaseURL : internalBaseURL;

    return url.replace(/\/$/, "");
}

export function filesUrlResolver() {
    const url = "http://127.0.0.1:8000/api/files/".replace(/\/$/, "")

    return {
        resolve: (uuid: string): string => `${url}/${uuid}`
    }
}

export function useApiUrlResolver() {
    const url = baseUrl();

    return {
        resolve: (path: string): string => `${url}/${path}`
    }
}