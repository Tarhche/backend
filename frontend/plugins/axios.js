import axios from "axios";
export default defineNuxtPlugin((nuxtApp)=>{
    nuxtApp.vueApp.use(axios)
    axios.defaults.baseURL="http://127.0.0.1:8000/api/"
})