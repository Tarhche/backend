<template>
  <div class="container">
    <div class="row mt-4">
      <article class="article col col-md-10 mx-auto">
        <dashboard-articles-create  @send="sendData"/>
      </article>
    </div>
  </div>
</template>
<script setup>
const router = useRouter()
definePageMeta({
  layout:"dashboard"
})
const {public: {baseURL}} = useRuntimeConfig()
 const  sendData = async (value)=>{
   const cookie = useCookie("tarche")
   const {data:data ,status, error}  = await useFetch( ()=>`${baseURL}/api/dashboard/articles` , {
    method:"post",
    headers:{
      Authorization: `Bearer ${cookie.value}`,
    },
    body:value
  })
  console.log(data.value , "data create")
   if (status.value === "success"){
     router.go(-1)
     await refreshNuxtData()
   }
  console.log(error.value , "error")
}

</script>
<style scoped>
.article{
  margin-top: 10vh;
}
</style>