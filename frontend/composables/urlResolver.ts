function baseUrl() {
    const {internalApiBaseUrl, public: p} = useRuntimeConfig()
    const url = process.client ? p.apiBaseUrl : internalApiBaseUrl;

    return {
        publicApiBaseUrl: p.apiBaseUrl && removeTrailingSlash(p.apiBaseUrl),
        internalApiBaseUrl: internalApiBaseUrl && removeTrailingSlash(internalApiBaseUrl),
        apiBaseUrl: url && removeTrailingSlash(url)
    };
}

function removeTrailingSlash(url: string): string {
    return url.replace(/\/$/, "")
}

export function useFilesUrlResolver() {
    const url = baseUrl().publicApiBaseUrl

    return {
        resolve: (uuid: string): string => `${url}/files/${uuid}`
    }
}

export function useApiUrlResolver() {
    const url = baseUrl().apiBaseUrl;

    return {
        resolve: (path: string): string => `${url}/${path}`
    }
}