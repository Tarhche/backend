function baseUrl() {
    const {internalApiBaseUrl, public: p} = useRuntimeConfig()
    const url = process.client ? p.apiBaseUrl : internalApiBaseUrl;

    return {
        publicApiBaseUrl: p.apiBaseUrl.replace(/\/$/, ""),
        internalApiBaseUrl: internalApiBaseUrl?.replace(/\/$/, ""),
        apiBaseUrl: url.replace(/\/$/, "")
    };
}

export function filesUrlResolver() {
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