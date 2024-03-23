<script setup lang="ts">
const {public: {baseURL}} = useRuntimeConfig()
const cookie = useCookie("tarche")
const showConfirm = ref(false)
const confirmDelete = ref(false)
const files = ref("")

  const {data: response, status, error} = await useAsyncData('files' ,()=>$fetch( `${baseURL}/api/dashboard/files`, {
    lazy: true,
    headers: {
      authorization: `Bearer ${cookie.value}`
    }
  }))
  if (response.value.items.length) {
    files.value = response.value.items
  }
  if (error.value) {
    console.log(error)
  }




function confirm() {
  confirmDelete.value = true
  showConfirm.value = false
}

function close() {
  showConfirm.value = false
}
</script>

<template>
<div class="container-lg">
  <div class="files-container">
    <div class="row">
      <transition name="transition">
        <modal-confirm @close="close" @confirm="confirm" v-if="showConfirm"/>
      </transition>
    </div>
    <div class="row">
      <!--      <loading-loader v-if="pendingData"/>-->
    </div>
    <div class="row">
      <article >
        <div class="files">
          <div class="card  " v-for="(item , index) in files" :key="index">
            <div class="card-header bg-white overflow-hidden p-0 h-75"><img class="w-100 h-100 " :src="`${baseURL}/files/${item.uuid}`" :alt="item.Name"></div>
            <div class="card-body ">
              <ul class="list-unstyled d-flex flex-column">
                <li><span class="name card-title text-muted">نام : {{item.Name}} </span></li>
                <li><span class="size card-title text-muted"> سایز عکس : {{item.Size}}</span></li>
              </ul>


            </div>
          </div>
        </div>
      </article>
    </div>
  </div>
</div>
</template>

<style scoped>
.files-container {
  min-height: calc(100vh - 200px);
}
.files{
  display: grid;
  grid-template-columns: repeat(4 , 1fr);
  gap: 1rem;
}
.card{
  cursor: pointer;
}

.card-header > img{
transition: 0.5s;
}
.card-header > img:hover{
  scale: 1.05;
}
.name , .size {
  font-size: 0.8rem;
}
.transition-enter-active {
  transition: all 0.7s ease;

}

.transition-leave-active {
  transition: all 0.5s ease;

}
.transition-enter-from, .transition-leave-to {
  opacity: 0;
  transform: translatey(-100%);
}
.transition-enter-to, .transition-leave-from {
  opacity: 1;
}
</style>