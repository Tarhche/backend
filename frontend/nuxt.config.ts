// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  css: [
    '~/assets/scss/main.scss',
  ],
  features: {
    inlineStyles: false,
  },
  modules: ['@vueuse/nuxt'],
  runtimeConfig: {
    internalApiBaseUrl: '',
    public: {
      apiBaseUrl: '',
    }
  },
  app:{
    head:{
      htmlAttrs:{
        dir: "rtl",
        lang: "fa",
      }
    }
  }
})
