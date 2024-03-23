<script setup lang="ts">
const {public: {baseURL}} = useRuntimeConfig()
const cookie = useCookie("tarche")
const showConfirm = ref(false)
const confirmDelete = ref(false)
const files = ref("")
const inputFile = ref(null)
const fileData = ref("")
const {data: response, pending, error , refresh} = await useAsyncData('files', () => $fetch(`${baseURL}/api/dashboard/files`, {
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

function deletePost(id) {
  showConfirm.value = true
  console.log(id)
  watch(confirmDelete, async () => {
    if (confirmDelete.value) {
      const {status, error} = await useAsyncData('delete', () => $fetch(`${baseURL}/api/dashboard/articles/${id}`, {
        method: "DELETE",
        headers: {
          Authorization: `Bearer ${cookie.value}`
        }
      }))
      if (status.value == "success") {
        console.log(status)
        await refreshNuxtData()
      }
      if (error.value) {
        console.log(error.value)
      }

    }
  })
  confirmDelete.value = false
}


function confirm() {
  confirmDelete.value = true
  showConfirm.value = false
}

function close() {
  showConfirm.value = false
}

function change() {
watch(inputFile , async ()=>{
  if (inputFile.value.files[0].length){
    const {status, error } = await useAsyncData('delete', () => $fetch(`${baseURL}/api/dashboard/articles`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${cookie.value}`
      },
      body: {
        file: inputFile.value.files[0]
      }
    }))

  if (status.value === "success") {
   await refresh()
  }

  console.log(error.value?.message)
  }
})

}
</script>

<template>
  <div class="container-sm">
    <div class="files-container">
      <div class="row">
        <transition name="transition">
          <modal-confirm @close="close" @confirm="confirm" v-if="showConfirm"/>
        </transition>
      </div>
      <div class="row ">
        <loading-loader v-if="pending"/>
      </div>
      <div class="row">
        <article>
          <div class="files ">
            <div class="card mb-0 " v-for="(item , index) in files" :key="index">
              <div class="card-header bg-white overflow-hidden p-0 h-75">
                <img class="w-100 h-100 "
                     :src="`${baseURL}/files/${item.uuid}`"
                     :alt="item.Name"></div>
              <div class="card-body ">
                <ul class="list-unstyled d-flex flex-column">
                  <li><span class="name card-title text-muted">نام : {{ item.Name }} </span></li>
                  <li><span class="size card-title text-muted"> سایز عکس : {{ item.Size }}</span></li>
                </ul>
              </div>
              <div class="card-footer bg-white border-0 px-1">
                <button class="btn btn-outline-danger btn-sm w-100" @click="deletePost(item.uuid)"> پاک کردن</button>
              </div>
            </div>
            <div class="add-files w-100 h-100">
              <div class="card border-primary border-2  w-100 h-100 ">
                <label for="file" class="card-body d-flex justify-content-center align-items-center h-100">
                  <i class="fa-regular fa-add fa-2xl dis"></i>
                  <input type="file" name="" @change="change" ref="inputFile" id="file">
                </label>
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

.files {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 1rem;
}

.add-files .card {
  transition: 0.5s;
  background-color: #7bed9f;
  color: #ffa502;
}

.add-files i {
  color: #ffa502;
}

.card {
  cursor: pointer;
}

.card-header {
  max-height: 150px;
}

.card-header > img {
  transition: 0.5s;
}

.card-header > img:hover {
  scale: 1.05;
}

.name, .size {
  font-size: 0.8rem;
}

#file {
  display: none;
}

label {
  cursor: pointer;
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

@media screen and (max-width: 1200px) {
  .files {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 1rem;
  }
}

@media screen and (max-width: 996px) {
  .files {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
  }
}

@media screen and (max-width: 768px) {
  .files {
    display: grid;
    grid-template-columns: repeat(1, 1fr);
  }
}
</style>