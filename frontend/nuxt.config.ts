// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: [
    "~/assets/scss/main.scss",
  ],
  modules: [
    '@vueuse/nuxt','@pinia/nuxt'
  ],
  runtimeConfig: {
    internalApiBaseUrl: process.env.INTERNAL_API_BASE_URL,
    public: {
      publicApiBaseUrl: process.env.API_BASE_URL,
    }
  }
})
