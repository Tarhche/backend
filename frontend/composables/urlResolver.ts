function baseUrl() {
    const {internalApiBaseUrl, public: p} = useRuntimeConfig()
    const url = process.client ? p.publicApiBaseUrl : internalApiBaseUrl;

    return {
        publicApiBaseUrl: p.publicApiBaseUrl.replace(/\/$/, ""),
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