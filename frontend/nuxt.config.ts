// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: [
    "~/assets/scss/main.scss",
  ],
  modules: [
    '@vueuse/nuxt',
  ],
  runtimeConfig: {
    internalApiBaseUrl: 'http://127.0.0.1:8000',
    public: {
      publicApiBaseUrl: 'http://127.0.0.1:8000',
    }
  }
})
